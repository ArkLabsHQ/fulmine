package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/cln"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/BoltzExchange/boltz-client/v2/pkg/boltz"
	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/bitcointree"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	grpcclient "github.com/ark-network/ark/pkg/client-sdk/client/grpc"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/ccoveille/go-safecast"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	log "github.com/sirupsen/logrus"
)

var boltzURLByNetwork = map[string]string{
	common.Bitcoin.Name:        "https://api.boltz.exchange/v2",
	common.BitcoinTestNet.Name: "https://api.testnet.boltz.exchange/v2",
	common.BitcoinRegTest.Name: "https://localhost:9001/v2",
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Service struct {
	BuildInfo BuildInfo

	arksdk.ArkClient
	storeRepo        types.Store
	settingsRepo     domain.SettingsRepository
	vhtlcRepo        domain.VHTLCRepository
	vtxoRolloverRepo domain.VtxoRolloverRepository
	grpcClient       client.TransportClient
	schedulerSvc     ports.SchedulerService
	lnSvc            ports.LnService
	boltzSvc         *boltz.Api

	publicKey *secp256k1.PublicKey

	esploraUrl string

	isReady bool

	subscriptions    map[string]string // tracks subscribed addresses (vtxo taproot pubkey -> address)
	subscriptionLock sync.RWMutex

	// Notification channels
	notifications chan Notification

	stopCh chan struct{}
}

type Notification struct {
	Address    string
	NewVtxos   []client.Vtxo
	SpentVtxos []client.Vtxo
}

func NewService(
	buildInfo BuildInfo,
	storeSvc types.Store,
	settingsRepo domain.SettingsRepository,
	vhtlcRepo domain.VHTLCRepository,
	vtxoRolloverRepo domain.VtxoRolloverRepository,
	schedulerSvc ports.SchedulerService,
	lnSvc ports.LnService,
	esploraUrl string,
) (*Service, error) {
	if arkClient, err := arksdk.LoadCovenantlessClient(storeSvc); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}
		grpcClient, err := grpcclient.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}
		svc := &Service{
			BuildInfo:        buildInfo,
			ArkClient:        arkClient,
			storeRepo:        storeSvc,
			settingsRepo:     settingsRepo,
			vhtlcRepo:        vhtlcRepo,
			vtxoRolloverRepo: vtxoRolloverRepo,
			grpcClient:       grpcClient,
			schedulerSvc:     schedulerSvc,
			lnSvc:            lnSvc,
			publicKey:        nil,
			isReady:          true,
			subscriptions:    make(map[string]string),
			subscriptionLock: sync.RWMutex{},
			notifications:    make(chan Notification),
			stopCh:           make(chan struct{}, 1),
			esploraUrl:       esploraUrl,
		}

		return svc, nil
	}

	ctx := context.Background()
	if _, err := settingsRepo.GetSettings(ctx); err != nil {
		if err := settingsRepo.AddDefaultSettings(ctx); err != nil {
			return nil, err
		}
	}
	arkClient, err := arksdk.NewCovenantlessClient(storeSvc)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	svc := &Service{
		BuildInfo:        buildInfo,
		ArkClient:        arkClient,
		storeRepo:        storeSvc,
		settingsRepo:     settingsRepo,
		vhtlcRepo:        vhtlcRepo,
		grpcClient:       nil,
		schedulerSvc:     schedulerSvc,
		lnSvc:            lnSvc,
		subscriptions:    make(map[string]string),
		subscriptionLock: sync.RWMutex{},
		notifications:    make(chan Notification),
		stopCh:           make(chan struct{}, 1),
		esploraUrl:       esploraUrl,
	}

	return svc, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) SetupFromMnemonic(ctx context.Context, serverUrl, password, mnemonic string) error {
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return s.Setup(ctx, serverUrl, password, privateKey)
}

func (s *Service) Setup(ctx context.Context, serverUrl, password, privateKey string) (err error) {
	privKeyBytes, err := hex.DecodeString(privateKey)
	if err != nil {
		return err
	}
	prvKey := secp256k1.PrivKeyFromBytes(privKeyBytes)

	client, err := grpcclient.NewClient(serverUrl)
	if err != nil {
		return err
	}

	if err := s.Init(ctx, arksdk.InitArgs{
		WalletType:          arksdk.SingleKeyWallet,
		ClientType:          arksdk.GrpcClient,
		ServerUrl:           serverUrl,
		ExplorerURL:         s.esploraUrl,
		Password:            password,
		Seed:                privateKey,
		WithTransactionFeed: true,
	}); err != nil {
		return err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	if err := s.settingsRepo.UpdateSettings(
		ctx, domain.Settings{ServerUrl: config.ServerUrl, EsploraUrl: config.ExplorerURL},
	); err != nil {
		return err
	}

	s.publicKey = prvKey.PubKey()
	s.grpcClient = client
	s.isReady = true
	return nil
}

func (s *Service) LockNode(ctx context.Context, password string) error {
	err := s.Lock(ctx, password)
	if err != nil {
		return err
	}
	s.schedulerSvc.Stop()
	log.Info("scheduler stopped")
	go func() {
		select {
		case <-s.stopCh:
			return
		default:
			time.Sleep(100 * time.Microsecond)
		}
	}()
	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	txCh, close, err := s.grpcClient.GetTransactionsStream(context.Background())
	if err != nil {
		return fmt.Errorf("server unreachable")
	}

	if err := s.Unlock(ctx, password); err != nil {
		return err
	}

	s.schedulerSvc.Start()
	log.Info("scheduler started")

	err = s.ScheduleClaims(ctx)
	if err != nil {
		log.WithError(err).Info("schedule next claim failed")
	}

	prvkeyStr, err := s.Dump(ctx)
	if err != nil {
		return err
	}

	buf, err := hex.DecodeString(prvkeyStr)
	if err != nil {
		return err
	}

	_, pubkey := btcec.PrivKeyFromBytes(buf)
	s.publicKey = pubkey

	settings, err := s.settingsRepo.GetSettings(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get settings")
		return err
	}
	if len(settings.LnUrl) > 0 {
		data, err := s.GetConfigData(ctx)
		if err != nil {
			return err
		}

		if strings.HasPrefix(settings.LnUrl, "clnconnect:") {
			s.lnSvc = cln.NewService()
		}
		if err := s.lnSvc.Connect(ctx, settings.LnUrl); err != nil {
			log.WithError(err).Warn("failed to connect to ln node")
		}
		boltzSvc := &boltz.Api{URL: boltzURLByNetwork[data.Network.Name]}
		s.boltzSvc = boltzSvc
	}

	go s.listenForNotifications(txCh, close)

	return nil
}

func (s *Service) Reset(ctx context.Context) error {
	backup, err := s.settingsRepo.GetSettings(ctx)
	if err != nil {
		return err
	}
	if err := s.settingsRepo.CleanSettings(ctx); err != nil {
		return err
	}
	if err := s.storeRepo.ConfigStore().CleanData(ctx); err != nil {
		// nolint:all
		s.settingsRepo.AddSettings(ctx, *backup)
		return err
	}
	return nil
}

func (s *Service) AddDefaultSettings(ctx context.Context) error {
	return s.settingsRepo.AddDefaultSettings(ctx)
}

func (s *Service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	sett, err := s.settingsRepo.GetSettings(ctx)
	return sett, err
}

func (s *Service) NewSettings(ctx context.Context, settings domain.Settings) error {
	return s.settingsRepo.AddSettings(ctx, settings)
}

func (s *Service) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	return s.settingsRepo.UpdateSettings(ctx, settings)
}

func (s *Service) GetAddress(ctx context.Context, sats uint64) (string, string, string, string, error) {
	offchainAddr, boardingAddr, err := s.Receive(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	bip21Addr := fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)
	// add amount if passed
	if sats > 0 {
		btc := float64(sats) / 100000000.0
		amount := fmt.Sprintf("%.8f", btc)
		bip21Addr += fmt.Sprintf("&amount=%s", amount)
	}
	pubkey := hex.EncodeToString(s.publicKey.SerializeCompressed())
	return bip21Addr, offchainAddr, boardingAddr, pubkey, nil
}

func (s *Service) GetTotalBalance(ctx context.Context) (uint64, error) {
	balance, err := s.Balance(ctx, false)
	if err != nil {
		return 0, err
	}
	onchainBalance := balance.OnchainBalance.SpendableAmount
	for _, amount := range balance.OnchainBalance.LockedAmount {
		onchainBalance += amount.Amount
	}
	return balance.OffchainBalance.Total + onchainBalance, nil
}

func (s *Service) GetRound(ctx context.Context, roundId string) (*client.Round, error) {
	if !s.isReady {
		return nil, fmt.Errorf("service not initialized")
	}
	return s.grpcClient.GetRoundByID(ctx, roundId)
}

func (s *Service) ClaimPending(ctx context.Context) (string, error) {
	roundTxid, err := s.ArkClient.Settle(ctx)
	if err == nil {
		err := s.ScheduleClaims(ctx)
		if err != nil {
			log.WithError(err).Warn("error scheduling next claims")
		}
	}
	return roundTxid, err
}

func (s *Service) ScheduleClaims(ctx context.Context) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	spendableVtxos, _, err := s.ArkClient.ListVtxos(ctx)
	if err != nil {
		return err
	}

	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	task := func() {
		log.Infof("running auto claim at %s", time.Now())
		_, err := s.ClaimPending(ctx)
		if err != nil {
			log.WithError(err).Warn("failed to auto claim")
		}
	}

	return s.schedulerSvc.ScheduleNextClaim(spendableVtxos, data, task)
}

func (s *Service) WhenNextClaim(ctx context.Context) (*time.Time, error) {
	return s.schedulerSvc.WhenNextClaim()
}

func (s *Service) ConnectLN(ctx context.Context, connectUrl string) error {
	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}
	boltzSvc := &boltz.Api{URL: boltzURLByNetwork[data.Network.Name]}

	if strings.HasPrefix(connectUrl, "clnconnect:") {
		s.lnSvc = cln.NewService()
	}
	if err := s.lnSvc.Connect(ctx, connectUrl); err != nil {
		return err
	}

	s.boltzSvc = boltzSvc
	return nil
}

func (s *Service) DisconnectLN() {
	s.lnSvc.Disconnect()
}

func (s *Service) IsConnectedLN() bool {
	return s.lnSvc.IsConnected()
}

func (s *Service) GetVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *secp256k1.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *common.AbsoluteLocktime,
	unilateralClaimDelayParam *common.RelativeLocktime,
	unilateralRefundDelayParam *common.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *common.RelativeLocktime,
) (string, *vhtlc.VHTLCScript, error) {
	receiverPubkeySet := receiverPubkey != nil
	senderPubkeySet := senderPubkey != nil
	if receiverPubkeySet == senderPubkeySet {
		return "", nil, fmt.Errorf("only one of receiver and sender pubkey must be set")
	}
	if !receiverPubkeySet {
		receiverPubkey = s.publicKey
	}
	if !senderPubkeySet {
		senderPubkey = s.publicKey
	}

	offchainAddr, _, err := s.Receive(ctx)
	if err != nil {
		return "", nil, err
	}

	decodedAddr, err := common.DecodeAddress(offchainAddr)
	if err != nil {
		return "", nil, err
	}

	// Default values if not provided
	refundLocktime := common.AbsoluteLocktime(80 * 600) // 80 blocks
	if refundLocktimeParam != nil {
		refundLocktime = *refundLocktimeParam
	}

	unilateralClaimDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 512, //60 * 12, // 12 hours
	}
	if unilateralClaimDelayParam != nil {
		unilateralClaimDelay = *unilateralClaimDelayParam
	}

	unilateralRefundDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 1024, //60 * 24, // 24 hours
	}
	if unilateralRefundDelayParam != nil {
		unilateralRefundDelay = *unilateralRefundDelayParam
	}

	unilateralRefundWithoutReceiverDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeBlock,
		Value: 224, // 224 blocks
	}
	if unilateralRefundWithoutReceiverDelayParam != nil {
		unilateralRefundWithoutReceiverDelay = *unilateralRefundWithoutReceiverDelayParam
	}

	opts := vhtlc.Opts{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		Server:                               decodedAddr.Server,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}
	vtxoScript, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", nil, err
	}

	tapKey, _, err := vtxoScript.TapTree()
	if err != nil {
		return "", nil, err
	}

	addr := &common.Address{
		HRP:        decodedAddr.HRP,
		Server:     decodedAddr.Server,
		VtxoTapKey: tapKey,
	}
	encodedAddr, err := addr.Encode()
	if err != nil {
		return "", nil, err
	}

	// store the vhtlc options for future use
	if err := s.vhtlcRepo.Add(ctx, opts); err != nil {
		return "", nil, err
	}

	return encodedAddr, vtxoScript, nil
}

func (s *Service) ListVHTLC(ctx context.Context, preimageHashFilter string) ([]client.Vtxo, []vhtlc.Opts, error) {
	// Get VHTLC options based on filter
	var vhtlcOpts []vhtlc.Opts
	if preimageHashFilter != "" {
		opt, err := s.vhtlcRepo.Get(ctx, preimageHashFilter)
		if err != nil {
			return nil, nil, err
		}
		vhtlcOpts = []vhtlc.Opts{*opt}
	} else {
		var err error
		vhtlcOpts, err = s.vhtlcRepo.GetAll(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	offchainAddr, _, err := s.Receive(ctx)
	if err != nil {
		return nil, nil, err
	}

	decodedAddr, err := common.DecodeAddress(offchainAddr)
	if err != nil {
		return nil, nil, err
	}

	var allVtxos []client.Vtxo
	for _, opt := range vhtlcOpts {
		vtxoScript, err := vhtlc.NewVHTLCScript(opt)
		if err != nil {
			return nil, nil, err
		}
		tapKey, _, err := vtxoScript.TapTree()
		if err != nil {
			return nil, nil, err
		}

		addr := &common.Address{
			HRP:        decodedAddr.HRP,
			Server:     decodedAddr.Server,
			VtxoTapKey: tapKey,
		}

		addrStr, err := addr.Encode()
		if err != nil {
			return nil, nil, err
		}

		// Get vtxos for this address
		vtxos, _, err := s.grpcClient.ListVtxos(ctx, addrStr)
		if err != nil {
			return nil, nil, err
		}
		allVtxos = append(allVtxos, vtxos...)
	}

	return allVtxos, vhtlcOpts, nil
}

func (s *Service) ClaimVHTLC(ctx context.Context, preimage []byte) (string, error) {
	preimageHash := hex.EncodeToString(btcutil.Hash160(preimage))

	vtxos, vhtlcOpts, err := s.ListVHTLC(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	if len(vtxos) == 0 {
		return "", fmt.Errorf("no vhtlc found")
	}

	vtxo := vtxos[0]
	opts := vhtlcOpts[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", err
	}

	claimClosure := vtxoScript.ClaimClosure
	claimWitnessSize := claimClosure.WitnessSize(len(preimage))
	claimScript, err := claimClosure.Script()
	if err != nil {
		return "", err
	}

	_, tapTree, err := vtxoScript.TapTree()
	if err != nil {
		return "", err
	}

	claimLeafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(claimScript).TapHash(),
	)
	if err != nil {
		return "", err
	}

	ctrlBlock, err := txscript.ParseControlBlock(claimLeafProof.ControlBlock)
	if err != nil {
		return "", err
	}

	// self send output
	_, myAddr, _, _, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", err
	}

	decodedAddr, err := common.DecodeAddress(myAddr)
	if err != nil {
		return "", err
	}

	pkScript, err := common.P2TRScript(decodedAddr.VtxoTapKey)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	redeemTx, err := bitcointree.BuildRedeemTx(
		[]common.VtxoInput{
			{
				Outpoint:    vtxoOutpoint,
				Amount:      amount,
				WitnessSize: claimWitnessSize,
				Tapscript: &waddrmgr.Tapscript{
					ControlBlock:   ctrlBlock,
					RevealedScript: claimScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
	)
	if err != nil {
		return "", err
	}

	redeemPtx, err := psbt.NewFromRawBytes(strings.NewReader(redeemTx), true)
	if err != nil {
		return "", err
	}

	if err := bitcointree.AddConditionWitness(0, redeemPtx, wire.TxWitness{preimage}); err != nil {
		return "", err
	}

	txid := redeemPtx.UnsignedTx.TxHash().String()

	redeemTx, err = redeemPtx.B64Encode()
	if err != nil {
		return "", err
	}

	signedRedeemTx, err := s.SignTransaction(ctx, redeemTx)
	if err != nil {
		return "", err
	}

	if _, _, err := s.grpcClient.SubmitRedeemTx(ctx, signedRedeemTx); err != nil {
		return "", err
	}

	return txid, nil
}

func (s *Service) RefundVHTLC(ctx context.Context, swapId, preimageHash string) (string, error) {
	vtxos, vhtlcOpts, err := s.ListVHTLC(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	if len(vtxos) == 0 {
		return "", fmt.Errorf("no vhtlc found")
	}

	vtxo := vtxos[0]
	opts := vhtlcOpts[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", err
	}

	refundClosure := vtxoScript.RefundClosure
	refundWitnessSize := refundClosure.WitnessSize()
	refundScript, err := refundClosure.Script()
	if err != nil {
		return "", err
	}

	_, tapTree, err := vtxoScript.TapTree()
	if err != nil {
		return "", err
	}

	refundLeafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(refundScript).TapHash(),
	)
	if err != nil {
		return "", err
	}

	ctrlBlock, err := txscript.ParseControlBlock(refundLeafProof.ControlBlock)
	if err != nil {
		return "", err
	}

	dest, err := txscript.PayToTaprootScript(opts.Sender)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	refundTx, err := bitcointree.BuildRedeemTx(
		[]common.VtxoInput{
			{
				Outpoint:    vtxoOutpoint,
				Amount:      amount,
				WitnessSize: refundWitnessSize,
				Tapscript: &waddrmgr.Tapscript{
					ControlBlock:   ctrlBlock,
					RevealedScript: refundScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: dest,
			},
		},
	)
	if err != nil {
		return "", err
	}

	refundPtx, err := psbt.NewFromRawBytes(strings.NewReader(refundTx), true)
	if err != nil {
		return "", err
	}

	txid := refundPtx.UnsignedTx.TxHash().String()

	refundTx, err = refundPtx.B64Encode()
	if err != nil {
		return "", err
	}

	signedRefundTx, err := s.SignTransaction(ctx, refundTx)
	if err != nil {
		return "", err
	}

	counterSignedRefundTx, err := s.boltzRefundSwap(swapId, refundTx, signedRefundTx, opts.Receiver)
	if err != nil {
		return "", err
	}

	if _, _, err := s.grpcClient.SubmitRedeemTx(ctx, counterSignedRefundTx); err != nil {
		return "", err
	}

	return txid, nil
}

func (s *Service) GetInvoice(ctx context.Context, amount uint64, memo, preimage string) (string, string, error) {
	return s.lnSvc.GetInvoice(ctx, amount, memo, preimage)
}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (string, error) {
	return s.lnSvc.PayInvoice(ctx, invoice)
}

func (s *Service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	return s.lnSvc.IsInvoiceSettled(ctx, invoice)
}

func (s *Service) GetBalanceLN(ctx context.Context) (msats uint64, err error) {
	return s.lnSvc.GetBalance(ctx)
}

// ln -> ark (reverse submarine swap)
func (s *Service) IncreaseInboundCapacity(ctx context.Context, amount uint64) (string, error) {
	// get our pubkey
	_, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get address: %s", err)
	}

	_, ph, err := s.GetInvoice(ctx, amount, "", "")
	if err != nil {
		return "", fmt.Errorf("failed to ger preimage hash: %s", err)
	}

	myPubkey, _ := hex.DecodeString(pk)
	preimageHash, _ := hex.DecodeString(ph)
	fromCurrency := boltz.Currency("LN")
	toCurrency := boltz.Currency("ARK")

	// make swap
	swap, err := s.boltzSvc.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           fromCurrency,
		To:             toCurrency,
		InvoiceAmount:  amount,
		OnchainAmount:  amount,
		ClaimPublicKey: boltz.HexString(myPubkey),
		PreimageHash:   boltz.HexString(preimageHash),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make reverse submarine swap: %v", err)
	}

	// verify vHTLC
	senderPubkey, err := parsePubkey(swap.RefundPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid refund pubkey: %v", err)
	}

	// TODO: fetch refundLocktimeParam, unilateralClaimDelayParam, unilateralRefundDelayParam, unilateralRefundWithoutReceiverDelayParam
	// from Boltz API response.
	vhtlcAddress, _, err := s.GetVHTLC(ctx, nil, senderPubkey, preimageHash, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// pay the invoice
	preimage, err := s.PayInvoice(ctx, swap.Invoice)
	if err != nil {
		return "", fmt.Errorf("failed to pay invoice: %v", err)
	}

	decodedPreimage, err := hex.DecodeString(preimage)
	if err != nil {
		return "", fmt.Errorf("invalid preimage: %v", err)
	}

	ws := s.boltzSvc.NewWebsocket()
	err = ws.Connect()
	for err != nil {
		log.WithError(err).Warn("failed to connect to boltz websocket")
		time.Sleep(time.Second)
		log.Debug("reconnecting...")
		err = ws.Connect()
	}

	err = ws.Subscribe([]string{swap.Id})
	for err != nil {
		log.WithError(err).Warn("failed to subscribe for swap events")
		time.Sleep(time.Second)
		log.Debug("retrying...")
		err = ws.Subscribe([]string{swap.Id})
	}

	var txid string
	for update := range ws.Updates {
		fmt.Printf("EVENT %+v\n", update)
		parsedStatus := boltz.ParseEvent(update.Status)

		switch parsedStatus {
		// TODO: ensure this is the right event to react to for claiming the vhtlc funded by Boltz.
		case boltz.TransactionMempool:
			txid, err = s.ClaimVHTLC(ctx, decodedPreimage)
			if err != nil {
				return "", fmt.Errorf("failed to claim vHTLC: %v", err)
			}
		}
		if txid != "" {
			break
		}
	}
	return txid, nil
}

// ark -> ln (submarine swap)
func (s *Service) IncreaseOutboundCapacity(ctx context.Context, amount uint64) (string, error) {
	// get our pubkey
	_, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get address: %v", err)
	}

	myPubkey, _ := hex.DecodeString(pk)

	// generate invoice where to receive funds
	invoice, preimageHash, err := s.GetInvoice(ctx, amount, "increase inbound capacity", "")
	if err != nil {
		return "", fmt.Errorf("failed to create invoice: %w", err)
	}

	decodedPreimageHash, err := hex.DecodeString(preimageHash)
	if err != nil {
		return "", fmt.Errorf("invalid preimage hash: %v", err)
	}

	fromCurrency := boltz.Currency("ARK")
	toCurrency := boltz.Currency("LN")
	// make swap
	swap, err := s.boltzSvc.CreateSwap(boltz.CreateSwapRequest{
		From:            fromCurrency,
		To:              toCurrency,
		Invoice:         invoice,
		RefundPublicKey: boltz.HexString(myPubkey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make submarine swap: %v", err)
	}

	// verify vHTLC
	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid claim pubkey: %v", err)
	}

	// TODO fetch refundLocktimeParam, unilateralClaimDelayParam, unilateralRefundDelayParam, unilateralRefundWithoutReceiverDelayParam from Boltz API
	address, _, err := s.GetVHTLC(ctx, receiverPubkey, nil, decodedPreimageHash, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != address {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// pay to vHTLC address
	receivers := []arksdk.Receiver{arksdk.NewBitcoinReceiver(swap.Address, amount)}
	txid, err := s.SendOffChain(ctx, false, receivers, true)
	if err != nil {
		return "", fmt.Errorf("failed to pay to vHTLC address: %v", err)
	}

	ws := s.boltzSvc.NewWebsocket()
	err = ws.Connect()
	for err != nil {
		log.WithError(err).Warn("failed to connect to boltz websocket")
		time.Sleep(time.Second)
		log.Debug("reconnecting...")
		err = ws.Connect()
	}

	err = ws.Subscribe([]string{swap.Id})
	for err != nil {
		log.WithError(err).Warn("failed to subscribe for swap events")
		time.Sleep(time.Second)
		log.Debug("retrying...")
		err = ws.Subscribe([]string{swap.Id})
	}

	for update := range ws.Updates {
		fmt.Printf("EVENT %+v\n", update)
		parsedStatus := boltz.ParseEvent(update.Status)

		switch parsedStatus {
		// TODO: ensure these are the right events to react to in case the vhtlc needs to be refunded.
		case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
			txid, err := s.RefundVHTLC(context.Background(), swap.Id, preimageHash)
			if err != nil {
				return "", fmt.Errorf("failed to refund vHTLC: %s", err)
			}

			return "", fmt.Errorf("something went wrong, the vhtlc was refunded %s", txid)
		case boltz.InvoiceSettled:
			return txid, nil
		}
	}

	return "", fmt.Errorf("something went wrong")
}

func (s *Service) SubscribeForAddresses(ctx context.Context, addresses []string) error {
	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	for _, addr := range addresses {
		decodedAddr, err := common.DecodeAddress(addr)
		if err != nil {
			return fmt.Errorf("invalid address: %s", err)
		}
		s.subscriptions[hex.EncodeToString(schnorr.SerializePubKey(decodedAddr.VtxoTapKey))] = addr
	}
	return nil
}

func (s *Service) UnsubscribeForAddresses(ctx context.Context, addresses []string) error {
	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	for _, addr := range addresses {
		delete(s.subscriptions, addr)
	}
	return nil
}

func (s *Service) GetVtxoNotifications(ctx context.Context) <-chan Notification {
	return s.notifications
}

func (s *Service) GetDelegatePublicKey(ctx context.Context) (string, error) {
	if s.publicKey == nil {
		return "", fmt.Errorf("service not initialized")
	}

	return hex.EncodeToString(s.publicKey.SerializeCompressed()), nil
}

func (s *Service) WatchAddressForRollover(ctx context.Context, address, destinationAddress string, taprootTree []string) error {
	if address == "" {
		return fmt.Errorf("missing address")
	}
	if len(taprootTree) == 0 {
		return fmt.Errorf("missing taproot tree")
	}
	if destinationAddress == "" {
		return fmt.Errorf("missing destination address")
	}

	target := domain.VtxoRolloverTarget{
		Address:            address,
		TaprootTree:        taprootTree,
		DestinationAddress: destinationAddress,
	}

	return s.vtxoRolloverRepo.AddTarget(ctx, target)
}

func (s *Service) UnwatchAddress(ctx context.Context, address string) error {
	if address == "" {
		return fmt.Errorf("missing address")
	}

	return s.vtxoRolloverRepo.RemoveTarget(ctx, address)
}

func (s *Service) ListWatchedAddresses(ctx context.Context) ([]domain.VtxoRolloverTarget, error) {
	return s.vtxoRolloverRepo.GetAllTargets(ctx)
}

func (s *Service) listenForNotifications(
	txCh <-chan client.TransactionEvent, closeFn func(),
) {
	emptyTx := client.TransactionEvent{}

	// listen for SDK vtxo channel events
	for {
		select {
		case <-s.stopCh:
			closeFn()
			return
		case tx := <-txCh:
			if tx == emptyTx {
				closeFn()
				return
			}

			notifications := make(map[string]Notification)
			var spendableVtxos []client.Vtxo
			var spentVtxos []client.Vtxo
			if tx.Round != nil {
				spendableVtxos = tx.Round.SpendableVtxos
				spentVtxos = tx.Round.SpentVtxos
			} else if tx.Redeem != nil {
				spendableVtxos = tx.Redeem.SpendableVtxos
				spentVtxos = tx.Redeem.SpentVtxos
			}

			s.subscriptionLock.RLock()
			for _, vtxo := range spendableVtxos {
				// check if the address is subscribed
				if address, ok := s.subscriptions[vtxo.PubKey]; ok {
					// check if the address is already in the notifications map
					if _, ok := notifications[address]; !ok {
						notifications[address] = Notification{
							Address:    address,
							NewVtxos:   make([]client.Vtxo, 0),
							SpentVtxos: make([]client.Vtxo, 0),
						}
					}

					n := notifications[address]
					n.NewVtxos = append(n.NewVtxos, vtxo)
					notifications[address] = n
				}
			}
			for _, vtxo := range spentVtxos {
				// check if the address is subscribed
				if address, ok := s.subscriptions[vtxo.PubKey]; ok {
					// check if the address is already in the notifications map
					if _, ok := notifications[address]; !ok {
						notifications[address] = Notification{
							Address:    address,
							NewVtxos:   make([]client.Vtxo, 0),
							SpentVtxos: make([]client.Vtxo, 0),
						}
					}

					n := notifications[address]
					n.SpentVtxos = append(n.SpentVtxos, vtxo)
					notifications[address] = n
				}
			}
			s.subscriptionLock.RUnlock()

			// send notifications through channel
			for _, notification := range notifications {
				go func() {
					select {
					case s.notifications <- notification:
					default:
						time.Sleep(100 * time.Millisecond)
					}
				}()
			}
		}
	}
}

func (s *Service) boltzRefundSwap(swapId, refundTx, signedRefundTx string, boltzPubkey *btcec.PublicKey) (string, error) {
	partialSig, err := s.boltzSvc.RefundSwap(swapId, &boltz.RefundRequest{
		Transaction: refundTx,
	})
	if err != nil {
		return "", err
	}
	sig, err := partialSig.PartialSignature.MarshalText()
	if err != nil {
		return "", err
	}

	ptx, _ := psbt.NewFromRawBytes(strings.NewReader(signedRefundTx), true)
	ptx.Inputs[0].TaprootScriptSpendSig = append(ptx.Inputs[0].TaprootScriptSpendSig, &psbt.TaprootScriptSpendSig{
		XOnlyPubKey: schnorr.SerializePubKey(boltzPubkey),
		LeafHash:    ptx.Inputs[0].TaprootScriptSpendSig[0].LeafHash,
		Signature:   sig,
		SigHash:     ptx.Inputs[0].TaprootScriptSpendSig[0].SigHash,
	})
	return ptx.B64Encode()
}

func parsePubkey(pubkey boltz.HexString) (*secp256k1.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	pk, err := secp256k1.ParsePubKey(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}
