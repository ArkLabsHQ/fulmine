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
	"github.com/ark-network/ark/pkg/client-sdk/explorer"
	"github.com/ark-network/ark/pkg/client-sdk/store"
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
	storeCfg         store.Config
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

	subscriptions    map[string]func() // tracks subscribed addresses (address -> closeFn)
	subscriptionLock sync.RWMutex

	// Notification channels
	notifications chan Notification

	stopBoardingEventListener chan struct{}
}

type Notification struct {
	Address    string
	NewVtxos   []client.Vtxo
	SpentVtxos []client.Vtxo
}

func NewService(
	buildInfo BuildInfo,
	storeCfg store.Config,
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
			BuildInfo:                 buildInfo,
			ArkClient:                 arkClient,
			storeCfg:                  storeCfg,
			storeRepo:                 storeSvc,
			settingsRepo:              settingsRepo,
			vhtlcRepo:                 vhtlcRepo,
			vtxoRolloverRepo:          vtxoRolloverRepo,
			grpcClient:                grpcClient,
			schedulerSvc:              schedulerSvc,
			lnSvc:                     lnSvc,
			publicKey:                 nil,
			isReady:                   true,
			subscriptions:             make(map[string]func()),
			subscriptionLock:          sync.RWMutex{},
			notifications:             make(chan Notification),
			esploraUrl:                esploraUrl,
			stopBoardingEventListener: make(chan struct{}),
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
		BuildInfo:                 buildInfo,
		ArkClient:                 arkClient,
		storeCfg:                  storeCfg,
		storeRepo:                 storeSvc,
		settingsRepo:              settingsRepo,
		vhtlcRepo:                 vhtlcRepo,
		grpcClient:                nil,
		schedulerSvc:              schedulerSvc,
		lnSvc:                     lnSvc,
		subscriptions:             make(map[string]func()),
		subscriptionLock:          sync.RWMutex{},
		notifications:             make(chan Notification),
		esploraUrl:                esploraUrl,
		stopBoardingEventListener: make(chan struct{}),
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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	err := s.Lock(ctx, password)
	if err != nil {
		return err
	}
	s.schedulerSvc.Stop()
	log.Info("scheduler stopped")

	// close all subscriptions
	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()
	for _, closeFn := range s.subscriptions {
		closeFn()
	}
	s.subscriptions = make(map[string]func())

	// close boarding event listener
	s.stopBoardingEventListener <- struct{}{}
	close(s.stopBoardingEventListener)
	s.stopBoardingEventListener = make(chan struct{})

	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	if err := s.Unlock(ctx, password); err != nil {
		return err
	}

	s.schedulerSvc.Start()
	log.Info("scheduler started")

	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	nextExpiry, err := s.computeNextExpiry(ctx, data)
	if err != nil {
		log.WithError(err).Error("failed to compute next expiry")
	}

	if nextExpiry != nil {
		if err := s.scheduleNextSettlement(ctx, *nextExpiry, data); err != nil {
			log.WithError(err).Info("schedule next claim failed")
		}
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
		if strings.HasPrefix(settings.LnUrl, "clnconnect:") {
			s.lnSvc = cln.NewService()
		}
		if err := s.lnSvc.Connect(ctx, settings.LnUrl); err != nil {
			log.WithError(err).Warn("failed to connect to ln node")
		}
		boltzSvc := &boltz.Api{URL: boltzURLByNetwork[data.Network.Name]}
		s.boltzSvc = boltzSvc
	}

	offchainAddress, onchainAddress, err := s.Receive(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get addresses")
		return err
	}

	if err := s.SubscribeForAddresses(ctx, []string{offchainAddress}); err != nil {
		log.WithError(err).Error("failed to subscribe for offchain address")
		return err
	}

	go s.subscribeForBoardingEvent(ctx, onchainAddress, data)

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

	s.Stop()
	// nolint:all
	storeSvc, _ := store.NewStore(s.storeCfg)
	// nolint:all
	cli, _ := arksdk.NewCovenantlessClient(storeSvc)
	// TODO: Maybe drop?
	// nolint:all
	s.settingsRepo.AddDefaultSettings(ctx)
	s.ArkClient = cli
	s.storeRepo = storeSvc
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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.settingsRepo.AddSettings(ctx, settings)
}

func (s *Service) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.settingsRepo.UpdateSettings(ctx, settings)
}

func (s *Service) GetAddress(ctx context.Context, sats uint64) (string, string, string, string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", "", "", err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

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
	return s.grpcClient.GetRoundByID(ctx, roundId)
}

func (s *Service) Settle(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	roundTxid, err := s.ArkClient.Settle(ctx)
	if err == nil {
		// if settlement was successful, schedule the next one at now+vtxo expiry
		now := time.Now()
		data, err := s.GetConfigData(ctx)
		if err != nil {
			return "", err
		}
		expiry := data.VtxoTreeExpiry.Seconds()
		nextSettlement := now.Add(time.Duration(expiry) * time.Second)
		if err := s.scheduleNextSettlement(ctx, nextSettlement, data); err != nil {
			log.WithError(err).Warn("error scheduling next claims")
		}
	}
	return roundTxid, err
}

func (s *Service) scheduleNextSettlement(ctx context.Context, at time.Time, data *types.Config) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	task := func() {
		log.Infof("running auto claim at %s", time.Now())
		_, err := s.Settle(ctx)
		if err != nil {
			log.WithError(err).Warn("failed to auto claim")
		}
	}

	return s.schedulerSvc.ScheduleNextSettlement(at, data, task)
}

func (s *Service) WhenNextSettlement(ctx context.Context) (*time.Time, error) {
	return s.schedulerSvc.WhenNextSettlement()
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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", nil, err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, nil, err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

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
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				WitnessSize:        claimWitnessSize,
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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

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
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				WitnessSize:        refundWitnessSize,
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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.GetInvoice(ctx, amount, memo, preimage)
}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.PayInvoice(ctx, invoice)
}

func (s *Service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return false, err
	}

	if !s.lnSvc.IsConnected() {
		return false, fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.IsInvoiceSettled(ctx, invoice)
}

func (s *Service) GetBalanceLN(ctx context.Context) (msats uint64, err error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

	if !s.lnSvc.IsConnected() {
		return 0, fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.GetBalance(ctx)
}

// ln -> ark (reverse submarine swap)
func (s *Service) IncreaseInboundCapacity(ctx context.Context, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	for _, addr := range addresses {
		eventsCh, closeFn, err := s.grpcClient.SubscribeForAddress(context.Background(), addr)
		if err != nil {
			return fmt.Errorf("server unreachable")
		}

		s.subscriptions[addr] = closeFn
		go s.handleAddressEventChannel(eventsCh, addr)
	}
	return nil
}

func (s *Service) UnsubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	for _, addr := range addresses {
		closeFn, ok := s.subscriptions[addr]
		if !ok {
			log.Warnf("address %s not subscribed, skipping unsubscription", addr)
			continue
		}
		closeFn()
		delete(s.subscriptions, addr)
	}
	return nil
}

func (s *Service) GetVtxoNotifications(ctx context.Context) <-chan Notification {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil
	}

	return s.notifications
}

func (s *Service) GetDelegatePublicKey(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	if s.publicKey == nil {
		return "", fmt.Errorf("service not initialized")
	}

	return hex.EncodeToString(s.publicKey.SerializeCompressed()), nil
}

func (s *Service) WatchAddressForRollover(ctx context.Context, address, destinationAddress string, taprootTree []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

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
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	if address == "" {
		return fmt.Errorf("missing address")
	}

	return s.vtxoRolloverRepo.RemoveTarget(ctx, address)
}

func (s *Service) ListWatchedAddresses(ctx context.Context) ([]domain.VtxoRolloverTarget, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	return s.vtxoRolloverRepo.GetAllTargets(ctx)
}

func (s *Service) isInitializedAndUnlocked(ctx context.Context) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	if s.IsLocked(ctx) {
		return fmt.Errorf("service is locked")
	}

	return nil
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

func (s *Service) computeNextExpiry(ctx context.Context, data *types.Config) (*time.Time, error) {
	spendableVtxos, _, err := s.ListVtxos(ctx)
	if err != nil {
		return nil, err
	}

	var expiry *time.Time

	if len(spendableVtxos) > 0 {
		nextExpiry := spendableVtxos[0].ExpiresAt
		for _, vtxo := range spendableVtxos[1:] {
			if vtxo.ExpiresAt.Before(nextExpiry) {
				nextExpiry = vtxo.ExpiresAt
			}
		}
		expiry = &nextExpiry
	}

	txs, err := s.GetTransactionHistory(ctx)
	if err != nil {
		return nil, err
	}

	// check for unsettled boarding UTXOs
	for _, tx := range txs {
		if len(tx.TransactionKey.BoardingTxid) > 0 && !tx.Settled {
			// TODO replace by boardingExitDelay https://github.com/ark-network/ark/pull/501
			boardingExpiry := tx.CreatedAt.Add(time.Duration(data.UnilateralExitDelay.Seconds()*2) * time.Second)
			if expiry == nil || boardingExpiry.Before(*expiry) {
				expiry = &boardingExpiry
			}
		}
	}

	return expiry, nil
}

// subscribeForBoardingEvent aims to update the scheduled settlement
// by checking for spent and new vtxos on the given boarding address
func (s *Service) subscribeForBoardingEvent(ctx context.Context, address string, cfg *types.Config) {
	ticker := time.NewTicker(time.Minute * 10)
	defer ticker.Stop()

	expl := explorer.NewExplorer(s.esploraUrl, cfg.Network)

	currentSet, err := expl.GetUtxos(address)
	if err != nil {
		log.WithError(err).Error("failed to get utxos")
		return
	}

	for {
		select {
		case <-s.stopBoardingEventListener:
			return
		case <-ticker.C:
			utxos, err := expl.GetUtxos(address)
			if err != nil {
				log.WithError(err).Error("failed to get utxos")
				continue
			}

			if len(utxos) == 0 {
				continue
			}

			// TODO: use boardingExitDelay https://github.com/ark-network/ark/pull/501
			boardingTimelock := common.RelativeLocktime{Type: cfg.UnilateralExitDelay.Type, Value: cfg.UnilateralExitDelay.Value * 2}

			// find spent utxos
			spentUtxos := make([]types.Utxo, 0)
			for _, oldSetUtxo := range currentSet {
				found := false
				for _, newSetUtxo := range utxos {
					if oldSetUtxo.Txid == newSetUtxo.Txid && oldSetUtxo.Vout == newSetUtxo.Vout {
						found = true
						break
					}
				}
				if !found {
					spentUtxos = append(spentUtxos, oldSetUtxo.ToUtxo(boardingTimelock, []string{}))
				}
			}

			// find new utxos
			newUtxos := make([]types.Utxo, 0)
			for _, newSetUtxo := range utxos {
				found := false
				for _, oldSetUtxo := range currentSet {
					if oldSetUtxo.Txid == newSetUtxo.Txid && oldSetUtxo.Vout == newSetUtxo.Vout {
						found = true
						break
					}
				}
				if !found {
					newUtxos = append(newUtxos, newSetUtxo.ToUtxo(boardingTimelock, []string{}))
				}
			}

			log.Infof("boarding event detected: %d utxos spent, %d new utxos", len(spentUtxos), len(newUtxos))

			// if some are spent, it means we had a boarding tx settled
			if len(spentUtxos) > 0 {
				nextExpiry, err := s.computeNextExpiry(ctx, cfg)
				if err != nil {
					log.WithError(err).Error("failed to compute next expiry")
					continue
				}

				if err := s.scheduleNextSettlement(ctx, *nextExpiry, cfg); err != nil {
					log.WithError(err).Info("schedule next claim failed")
				}
				continue
			}

			// if some are new, it means we have a new boarding tx
			// if expiry is before the next scheduled settlement, we need to schedule a new one
			if len(newUtxos) > 0 {
				nextScheduledSettlement, _ := s.WhenNextSettlement(ctx)

				needSchedule := false

				for _, vtxo := range newUtxos {
					if nextScheduledSettlement == nil || vtxo.SpendableAt.Before(*nextScheduledSettlement) {
						nextScheduledSettlement = &vtxo.SpendableAt
						needSchedule = true
					}
				}

				if needSchedule {
					if err := s.scheduleNextSettlement(ctx, *nextScheduledSettlement, cfg); err != nil {
						log.WithError(err).Info("schedule next claim failed")
					}
				}
			}

			// update current set
			currentSet = utxos
		}
	}

}

func (s *Service) handleAddressEventChannel(eventsCh <-chan client.AddressEvent, addr string) {
	for event := range eventsCh {
		if event.Err != nil {
			log.WithError(event.Err).Error("AddressEvent subscription error")
			continue
		}

		log.Infof("received address event for %s (%d spent vtxos, %d new vtxos)", addr, len(event.SpentVtxos), len(event.NewVtxos))

		// non-blocking forward to notifications channel
		go func() {
			s.notifications <- Notification{
				Address:    addr,
				NewVtxos:   event.NewVtxos,
				SpentVtxos: event.SpentVtxos,
			}
		}()

		ctx := context.Background()

		data, err := s.GetConfigData(ctx)
		if err != nil {
			log.WithError(err).Error("failed to get config data")
			return
		}

		// if some vtxos were spent, schedule a settlement to soonest expiry among new vtxos / boarding UTXOs set
		if len(event.SpentVtxos) > 0 {
			nextExpiry, err := s.computeNextExpiry(ctx, data)
			if err != nil {
				log.WithError(err).Error("failed to compute next expiry")
				return
			}

			if err := s.scheduleNextSettlement(ctx, *nextExpiry, data); err != nil {
				log.WithError(err).Info("schedule next claim failed")
			}

			return
		}

		// if some vtxos were created, schedule a settlement to the soonest expiry among new vtxos
		if len(event.NewVtxos) > 0 {
			nextScheduledSettlement, _ := s.WhenNextSettlement(ctx)

			needSchedule := false

			for _, vtxo := range event.NewVtxos {
				if nextScheduledSettlement == nil || vtxo.ExpiresAt.Before(*nextScheduledSettlement) {
					nextScheduledSettlement = &vtxo.ExpiresAt
					needSchedule = true
				}
			}

			if needSchedule {
				if err := s.scheduleNextSettlement(ctx, *nextScheduledSettlement, data); err != nil {
					log.WithError(err).Info("schedule next claim failed")
				}
			}
		}
	}
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
