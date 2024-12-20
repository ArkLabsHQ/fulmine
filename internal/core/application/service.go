package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/ArkLabsHQ/ark-node/pkg/vhtlc"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/bitcointree"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	grpcclient "github.com/ark-network/ark/pkg/client-sdk/client/grpc"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/ccoveille/go-safecast"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/sirupsen/logrus"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Service struct {
	BuildInfo BuildInfo

	arksdk.ArkClient
	storeRepo    types.Store
	settingsRepo domain.SettingsRepository
	vhtlcRepo    domain.VHTLCRepository
	grpcClient   client.TransportClient
	schedulerSvc ports.SchedulerService
	lnSvc        ports.LnService

	publicKey *secp256k1.PublicKey

	isReady bool
}

func NewService(
	buildInfo BuildInfo,
	storeSvc types.Store,
	settingsRepo domain.SettingsRepository,
	vhtlcRepo domain.VHTLCRepository,
	schedulerSvc ports.SchedulerService,
	lnSvc ports.LnService,
) (*Service, error) {
	if arkClient, err := arksdk.LoadCovenantlessClient(storeSvc); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}
		client, err := grpcclient.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}
		return &Service{
			buildInfo, arkClient, storeSvc, settingsRepo, vhtlcRepo, client, schedulerSvc, lnSvc, nil, true,
		}, nil
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

	return &Service{buildInfo, arkClient, storeSvc, settingsRepo, vhtlcRepo, nil, schedulerSvc, lnSvc, nil, false}, nil
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

	if err := s.settingsRepo.UpdateSettings(
		ctx, domain.Settings{ServerUrl: serverUrl},
	); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			// nolint:all
			s.settingsRepo.UpdateSettings(ctx, domain.Settings{ServerUrl: ""})
		}
	}()

	client, err := grpcclient.NewClient(serverUrl)
	if err != nil {
		return err
	}

	if err := s.Init(ctx, arksdk.InitArgs{
		WalletType: arksdk.SingleKeyWallet,
		ClientType: arksdk.GrpcClient,
		ServerUrl:  serverUrl,
		Password:   password,
		Seed:       privateKey,
	}); err != nil {
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
	logrus.Info("scheduler stopped")
	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	err := s.Unlock(ctx, password)
	if err != nil {
		return err
	}

	s.schedulerSvc.Start()
	logrus.Info("scheduler started")

	err = s.ScheduleClaims(ctx)
	if err != nil {
		logrus.WithError(err).Info("schedule next claim failed")
	}

	go func() {
		settings, err := s.settingsRepo.GetSettings(ctx)
		if err != nil {
			logrus.WithError(err).Warn("failed to get settings")
			return
		}
		if len(settings.LnUrl) > 0 {
			if err := s.lnSvc.Connect(ctx, settings.LnUrl); err != nil {
				logrus.WithError(err).Warn("failed to connect")
			}
		}
	}()

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

func (s *Service) GetAddress(
	ctx context.Context, sats uint64,
) (string, string, string, string, error) {
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
	pubkey := hex.EncodeToString(s.publicKey.SerializeCompressed()[1:])
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
			logrus.WithError(err).Warn("error scheduling next claims")
		}
	}
	return roundTxid, err
}

func (s *Service) ScheduleClaims(ctx context.Context) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	txHistory, err := s.ArkClient.GetTransactionHistory(ctx)
	if err != nil {
		return err
	}

	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	task := func() {
		logrus.Infof("running auto claim at %s", time.Now())
		_, err := s.ClaimPending(ctx)
		if err != nil {
			logrus.WithError(err).Warn("failed to auto claim")
		}
	}

	return s.schedulerSvc.ScheduleNextClaim(txHistory, data, task)
}

func (s *Service) WhenNextClaim(ctx context.Context) time.Time {
	return s.schedulerSvc.WhenNextClaim()
}

func (s *Service) ConnectLN(ctx context.Context, lndconnectUrl string) error {
	return s.lnSvc.Connect(ctx, lndconnectUrl)
}

func (s *Service) DisconnectLN() {
	s.lnSvc.Disconnect()
}

func (s *Service) IsConnectedLN() bool {
	return s.lnSvc.IsConnected()
}

func (s *Service) GetVHTLC(
	ctx context.Context, receiverPubkey, senderPubkey *secp256k1.PublicKey, preimageHash []byte,
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

	// TODO: make these delays configurable
	refundLocktime := common.AbsoluteLocktime(80 * 600) // 80 blocks
	unilateralClaimDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 60 * 12, // 12 hours
	}
	unilateralRefundDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 60 * 24, // 24 hours
	}
	unilateralRefundWithoutReceiverDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeBlock,
		Value: 224, // 224 blocks
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

	witnessSize, err := safecast.ToUint64(claimWitnessSize)
	if err != nil {
		return "", err
	}

	weightEstimator := &input.TxWeightEstimator{}
	weightEstimator.AddTapscriptInput(
		lntypes.VByte(witnessSize).ToWU(),
		&waddrmgr.Tapscript{
			ControlBlock:   ctrlBlock,
			RevealedScript: claimScript,
		},
	)
	weightEstimator.AddP2TROutput()

	size, err := safecast.ToUint64(weightEstimator.VSize())
	if err != nil {
		return "", err
	}

	// TODO better fee rate
	sats := chainfee.AbsoluteFeePerKwFloor.FeeForVByte(lntypes.VByte(size)).ToUnit(btcutil.AmountSatoshi)
	fees, err := safecast.ToInt64(sats)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	if amount < fees {
		return "", fmt.Errorf("fees are greater than vhtlc amount %d < %d", vtxo.Amount, fees)
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
				Value:    amount - fees,
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

	// TODO: expose wallet's signer in ark sdk
	// TODO: sign redeemTx
	redeemTx, err = redeemPtx.B64Encode()
	if err != nil {
		return "", err
	}

	if _, err := s.grpcClient.SubmitRedeemTx(ctx, redeemTx); err != nil {
		return "", err
	}

	return txid, nil
}

func (s *Service) GetInvoice(
	ctx context.Context, amount uint64, memo string,
) (string, string, error) {
	return s.lnSvc.GetInvoice(ctx, amount, memo)
}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (string, error) {
	return s.lnSvc.PayInvoice(ctx, invoice)
}
