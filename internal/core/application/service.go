package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/cln"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/lnd"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	indexer "github.com/arkade-os/go-sdk/indexer"
	indexerTransport "github.com/arkade-os/go-sdk/indexer/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

const (
	WalletInit                                  = "init"
	WalletUnlock                                = "unlock"
	WalletReset                                 = "reset"
	defaultUnilateralClaimDelay                 = 512
	defaultUnilateralRefundDelay                = 1024
	defaultUnilateralRefundWithoutReceiverDelay = 2048
	defaultRefundLocktime                       = time.Hour * 24
)

var ErrorNoVtxosFound = fmt.Errorf("no vtxos found for the given vhtlc opts")

var boltzURLByNetwork = map[string]string{
	arklib.Bitcoin.Name:          "https://api.boltz.exchange",
	arklib.BitcoinTestNet.Name:   "https://api.testnet.boltz.exchange",
	arklib.BitcoinMutinyNet.Name: "https://api.boltz.mutinynet.arkade.sh",
	arklib.BitcoinRegTest.Name:   "http://localhost:9001",
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type WalletUpdate struct {
	Type     string
	Password string
}

type Service struct {
	BuildInfo     BuildInfo
	IndexerClient indexer.Indexer

	arksdk.ArkClient
	storeCfg     store.Config
	storeRepo    types.Store
	dbSvc        ports.RepoManager
	grpcClient   client.TransportClient
	schedulerSvc ports.SchedulerService
	lnSvc        ports.LnService
	boltzSvc     *boltz.Api

	publicKey *btcec.PublicKey

	esploraUrl string
	boltzUrl   string
	boltzWSUrl string

	swapTimeout uint32

	isReady bool

	internalSubscription *subscriptionHandler
	externalSubscription *subscriptionHandler

	walletUpdates chan WalletUpdate

	// Notification channels
	notifications chan Notification

	stopBoardingEventListener chan struct{}
}

type Notification struct {
	indexer.TxData
	Addrs       []string
	NewVtxos    []types.Vtxo
	SpentVtxos  []types.Vtxo
	Checkpoints map[string]indexer.TxData
}

type SwapResponse struct {
	TxId       string
	SwapStatus domain.SwapStatus
	Invoice    string
}

func NewService(
	buildInfo BuildInfo,
	storeCfg store.Config,
	storeSvc types.Store,
	dbSvc ports.RepoManager,
	schedulerSvc ports.SchedulerService,
	esploraUrl, boltzUrl, boltzWSUrl string, swapTimeout uint32,
	connectionOpts *domain.LnConnectionOpts,
) (*Service, error) {
	opts := make([]arksdk.ClientOption, 0)
	if log.IsLevelEnabled(log.DebugLevel) {
		opts = append(opts, arksdk.WithVerbose())
	}

	if arkClient, err := arksdk.LoadArkClient(storeSvc, opts...); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}

		grpcClient, err := grpcclient.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}

		indexerClient, err := indexerTransport.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}

		svc := &Service{
			BuildInfo:                 buildInfo,
			ArkClient:                 arkClient,
			storeCfg:                  storeCfg,
			storeRepo:                 storeSvc,
			dbSvc:                     dbSvc,
			grpcClient:                grpcClient,
			IndexerClient:             indexerClient,
			schedulerSvc:              schedulerSvc,
			publicKey:                 nil,
			isReady:                   true,
			notifications:             make(chan Notification),
			stopBoardingEventListener: make(chan struct{}),
			esploraUrl:                data.ExplorerURL,
			boltzUrl:                  boltzUrl,
			boltzWSUrl:                boltzWSUrl,
			swapTimeout:               swapTimeout,
			walletUpdates:             make(chan WalletUpdate),
		}

		return svc, nil
	} else if !strings.Contains(err.Error(), "not initialized") {
		return nil, err
	}

	ctx := context.Background()
	settingsRepo := dbSvc.Settings()
	if _, err := settingsRepo.GetSettings(ctx); err != nil {
		if err := settingsRepo.AddDefaultSettings(ctx); err != nil {
			return nil, err
		}
	}

	arkClient, err := arksdk.NewArkClient(storeSvc, opts...)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	if connectionOpts != nil {
		if err := dbSvc.Settings().UpdateSettings(ctx, domain.Settings{
			LnConnectionOpts: connectionOpts,
		}); err != nil {
			return nil, err
		}
	}

	svc := &Service{
		BuildInfo:                 buildInfo,
		ArkClient:                 arkClient,
		storeCfg:                  storeCfg,
		storeRepo:                 storeSvc,
		dbSvc:                     dbSvc,
		grpcClient:                nil,
		schedulerSvc:              schedulerSvc,
		notifications:             make(chan Notification),
		stopBoardingEventListener: make(chan struct{}),
		esploraUrl:                esploraUrl,
		boltzUrl:                  boltzUrl,
		boltzWSUrl:                boltzWSUrl,
		swapTimeout:               swapTimeout,
		walletUpdates:             make(chan WalletUpdate),
	}

	return svc, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) GetWalletUpdates() <-chan WalletUpdate {
	return s.walletUpdates
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
	prvKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)

	validatedServerUrl, err := utils.ValidateURL(serverUrl)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	client, err := grpcclient.NewClient(validatedServerUrl)
	if err != nil {
		return err
	}

	indexerClient, err := indexerTransport.NewClient(validatedServerUrl)
	if err != nil {
		return err
	}

	infos, err := client.GetInfo(ctx)
	if err != nil {
		return err
	}

	pollingInterval := 5 * time.Minute
	if infos.Network == "regtest" {
		log.Info("using faster polling interval for regtest")
		pollingInterval = 5 * time.Second
	}

	if err := s.Init(ctx, arksdk.InitArgs{
		WalletType:           arksdk.SingleKeyWallet,
		ClientType:           arksdk.GrpcClient,
		ServerUrl:            validatedServerUrl,
		ExplorerURL:          s.esploraUrl,
		ExplorerPollInterval: pollingInterval,
		Password:             password,
		Seed:                 privateKey,
		WithTransactionFeed:  true,
	}); err != nil {
		return err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	if err := s.dbSvc.Settings().UpdateSettings(
		ctx, domain.Settings{ServerUrl: config.ServerUrl, EsploraUrl: config.ExplorerURL},
	); err != nil {
		return err
	}

	url := s.boltzUrl
	wsUrl := s.boltzWSUrl
	if url == "" {
		url = boltzURLByNetwork[config.Network.Name]
	}
	if wsUrl == "" {
		wsUrl = boltzURLByNetwork[config.Network.Name]
	}
	s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}

	s.esploraUrl = config.ExplorerURL
	s.publicKey = prvKey.PubKey()
	s.grpcClient = client
	s.IndexerClient = indexerClient
	s.isReady = true

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletInit, Password: password}
	}()

	return nil
}

func (s *Service) LockNode(ctx context.Context) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	err := s.Lock(ctx)
	if err != nil {
		return err
	}

	s.schedulerSvc.Stop()
	log.Info("scheduler stopped")

	s.internalSubscription.stop()
	s.externalSubscription.stop()

	// close boarding event listener
	s.stopBoardingEventListener <- struct{}{}
	close(s.stopBoardingEventListener)
	s.stopBoardingEventListener = make(chan struct{})

	go func() {
		s.walletUpdates <- WalletUpdate{Type: "lock"}
	}()

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

	arkConfig, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	nextExpiry, err := s.computeNextExpiry(ctx, arkConfig)
	if err != nil {
		log.WithError(err).Error("failed to compute next expiry")
	}

	if nextExpiry != nil {
		if err := s.scheduleNextSettlement(*nextExpiry, arkConfig); err != nil {
			log.WithError(err).Error("failed to schedule next settlement")
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

	settings, err := s.dbSvc.Settings().GetSettings(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get settings")
		return err
	}

	if settings.LnConnectionOpts != nil {
		log.Debug("connecting to LN node...")
		if err = s.connectLN(ctx, settings.LnConnectionOpts); err != nil {
			log.WithError(err).Error("failed to connect to LN node")
			return err
		}
	}

	url := s.boltzUrl
	wsUrl := s.boltzWSUrl
	if url == "" {
		url = boltzURLByNetwork[arkConfig.Network.Name]
	}
	if wsUrl == "" {
		wsUrl = boltzURLByNetwork[arkConfig.Network.Name]
	}
	s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}

	go s.resumePendingSwapRefunds(context.Background())

	_, offchainAddress, boardingAddr, err := s.Receive(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get addresses")
		return err
	}

	offchainPkScript, err := offchainAddressPkScript(offchainAddress)
	if err != nil {
		log.WithError(err).Error("failed to get offchain address")
		return err
	}

	s.internalSubscription = newSubscriptionHandler(
		settings.ServerUrl,
		internalScriptsStore(offchainPkScript),
		s.handleInternalAddressEventChannel,
	)

	s.externalSubscription = newSubscriptionHandler(
		settings.ServerUrl,
		s.dbSvc.SubscribedScript(),
		s.handleAddressEventChannel(arkConfig),
	)

	if err := s.internalSubscription.start(); err != nil {
		log.WithError(err).Error("failed to start internal subscription")
		return err
	}
	if err := s.externalSubscription.start(); err != nil {
		log.WithError(err).Error("failed to start external subscription")
		return err
	}

	if arkConfig.UtxoMaxAmount != 0 {
		go s.subscribeForBoardingEvent(ctx, boardingAddr, arkConfig)
	}

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletUnlock, Password: password}
	}()

	return nil
}

func (s *Service) ResetWallet(ctx context.Context) error {
	if err := s.dbSvc.Settings().CleanSettings(ctx); err != nil {
		return err
	}
	// reset wallet (cleans all repos)
	s.Reset(ctx)
	// TODO: Maybe drop?
	// nolint:all
	s.dbSvc.Settings().AddDefaultSettings(ctx)

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletReset}
	}()
	return nil
}

func (s *Service) AddDefaultSettings(ctx context.Context) error {
	return s.dbSvc.Settings().AddDefaultSettings(ctx)
}

func (s *Service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	sett, err := s.dbSvc.Settings().GetSettings(ctx)
	return sett, err
}

func (s *Service) NewSettings(ctx context.Context, settings domain.Settings) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.dbSvc.Settings().AddSettings(ctx, settings)
}

func (s *Service) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.dbSvc.Settings().UpdateSettings(ctx, settings)
}

func (s *Service) GetAddress(ctx context.Context, sats uint64) (string, string, string, string, string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", "", "", "", err
	}

	var invoice string
	_, offchainAddr, boardingAddr, err := s.Receive(ctx)
	if err != nil {
		return "", "", "", "", "", err
	}

	bip21Addr := fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)

	invoiceResponse, err := s.GetInvoice(ctx, sats)

	invoice = invoiceResponse.Invoice

	if err == nil && len(invoice) > 0 {
		bip21Addr += fmt.Sprintf("&lightning=%s", invoice)
	}

	// add amount if passed
	if sats > 0 {
		btc := float64(sats) / 100000000.0
		amount := fmt.Sprintf("%.8f", btc)
		bip21Addr += fmt.Sprintf("&amount=%s", amount)
	}
	pubkey := hex.EncodeToString(s.publicKey.SerializeCompressed())
	return bip21Addr, offchainAddr, boardingAddr, invoice, pubkey, nil
}

func (s *Service) GetTotalBalance(ctx context.Context) (uint64, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

	balance, err := s.Balance(ctx, false)
	if err != nil {
		return 0, err
	}

	return balance.OffchainBalance.Total, nil
}

func (s *Service) GetRound(ctx context.Context, roundId string) (*indexer.CommitmentTx, error) {
	return s.IndexerClient.GetCommitmentTx(ctx, roundId)
}

func (s *Service) GetVirtualTxs(ctx context.Context, txids []string) ([]string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	resp, err := s.IndexerClient.GetVirtualTxs(ctx, txids)
	if err != nil {
		return nil, err
	}

	return resp.Txs, nil
}

func (s *Service) Settle(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	return s.ArkClient.Settle(ctx)
}

func (s *Service) scheduleNextSettlement(at time.Time, data *types.Config) error {
	task := func() {
		_, err := s.Settle(context.Background())
		if err != nil {
			log.WithError(err).Warn("failed to auto claim")
		}
	}

	// TODO: Fetch GetInfo to know the next market hour start, if any, and schedule the
	// settlement for the one closest to the vtxo expiry.

	sessionDuration := time.Duration(data.SessionDuration) * time.Second
	at = at.Add(-2 * sessionDuration) // schedule 2 rounds before the expiry

	return s.schedulerSvc.ScheduleNextSettlement(at, task)
}

func (s *Service) WhenNextSettlement(ctx context.Context) time.Time {
	return s.schedulerSvc.WhenNextSettlement()
}

func (s *Service) ConnectLN(ctx context.Context, lnUrl string) error {
	if len(lnUrl) == 0 {
		settings, err := s.dbSvc.Settings().GetSettings(ctx)
		if err != nil {
			log.WithError(err).Warn("failed to get settings")
			return err
		}

		if settings.LnConnectionOpts == nil {
			return fmt.Errorf("no LN connection options found, please provide a valid LN Connect URL")
		}

		return s.connectLN(ctx, settings.LnConnectionOpts)
	}

	if s.IsPreConfiguredLN() {
		return fmt.Errorf("cannot change LN URL, it is already pre-configured")
	}

	lnConnectionType := domain.CLN_CONNECTION
	if strings.Contains(lnUrl, "lndconnect:") {
		lnConnectionType = domain.LND_CONNECTION
	}

	lnConnctionOpts := &domain.LnConnectionOpts{
		LnUrl:          lnUrl,
		LnDatadir:      "",
		ConnectionType: lnConnectionType,
	}

	err := s.connectLN(ctx, lnConnctionOpts)
	if err != nil {
		return fmt.Errorf("failed to connect to LN node: %w", err)
	}

	err = s.dbSvc.Settings().UpdateSettings(ctx, domain.Settings{
		LnConnectionOpts: lnConnctionOpts,
	})
	if err != nil {
		return fmt.Errorf("failed to update LN connection options: %w", err)
	}

	return nil
}

func (s *Service) DisconnectLN() {
	s.lnSvc.Disconnect()
}

func (s *Service) IsConnectedLN() bool {
	if s.lnSvc == nil {
		return false
	}
	return s.lnSvc.IsConnected()
}

func (s *Service) GetLnConnectUrl() string {
	if s.lnSvc == nil {
		return ""
	}
	return s.lnSvc.GetLnConnectUrl()
}

func (s *Service) connectLN(ctx context.Context, lnOpts *domain.LnConnectionOpts) error {
	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	connectionOpts := lnOpts
	if connectionOpts.ConnectionType == domain.CLN_CONNECTION {
		s.lnSvc = cln.NewService()
	} else {
		s.lnSvc = lnd.NewService()
	}

	return s.lnSvc.Connect(ctx, connectionOpts, data.Network.Name)
}

func (s *Service) IsPreConfiguredLN() bool {
	settings, err := s.dbSvc.Settings().GetSettings(context.Background())
	if err != nil {
		return false
	}

	lnOpts := settings.LnConnectionOpts

	return lnOpts != nil && lnOpts.LnDatadir != ""
}

func (s *Service) GetSwapVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *arklib.AbsoluteLocktime,
	unilateralClaimDelayParam *arklib.RelativeLocktime,
	unilateralRefundDelayParam *arklib.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *arklib.RelativeLocktime,
) (string, string, *vhtlc.VHTLCScript, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", nil, err
	}

	receiverKey := receiverPubkey
	senderKey := senderPubkey

	if receiverKey == nil {
		receiverKey = s.publicKey
	}
	if senderKey == nil {
		senderKey = s.publicKey
	}

	compressedReceiverPubkey := receiverKey.SerializeCompressed()
	compressedSenderPubkey := senderKey.SerializeCompressed()
	vhtlcId := domain.GetVhtlcId(preimageHash, compressedSenderPubkey, compressedReceiverPubkey)

	if _, err := s.dbSvc.VHTLC().Get(ctx, vhtlcId); err == nil {
		return "", "", nil, fmt.Errorf("vHTLC with id %s already exists", vhtlcId)
	}

	// nolint
	cfg, _ := s.GetConfigData(ctx)

	// Default values if not provided
	refundLocktime := arklib.AbsoluteLocktime(time.Now().Add(defaultRefundLocktime).Unix())
	if refundLocktimeParam != nil {
		refundLocktime = *refundLocktimeParam
	}

	unilateralClaimDelay := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: defaultUnilateralClaimDelay, //60 * 12, // 12 hours
	}
	if unilateralClaimDelayParam != nil {
		unilateralClaimDelay = *unilateralClaimDelayParam
	}

	unilateralRefundDelay := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: defaultUnilateralRefundDelay, //60 * 24, // 24 hours
	}
	if unilateralRefundDelayParam != nil {
		unilateralRefundDelay = *unilateralRefundDelayParam
	}

	unilateralRefundWithoutReceiverDelay := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeBlock,
		Value: defaultUnilateralRefundWithoutReceiverDelay, // 224 blocks
	}
	if unilateralRefundWithoutReceiverDelayParam != nil {
		unilateralRefundWithoutReceiverDelay = *unilateralRefundWithoutReceiverDelayParam
	}

	opts := vhtlc.Opts{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		Server:                               cfg.SignerPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}
	vHTLCScript, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", "", nil, err
	}

	encodedAddr, err := vHTLCScript.Address(cfg.Network.Addr, cfg.SignerPubKey)
	if err != nil {
		return "", "", nil, err
	}

	if err != nil {
		return "", "", nil, err
	}

	go func() {
		if err := s.dbSvc.VHTLC().Add(context.Background(), domain.NewVhtlc(opts)); err != nil {
			log.WithError(err).Error("failed to add vhtlc")
			return
		}

		log.Debugf("added new vhtlc %s", vhtlcId)
	}()

	return encodedAddr, vhtlcId, vHTLCScript, nil
}

func (s *Service) ListSwapVHTLC(ctx context.Context, vhtlc_id string) ([]types.Vtxo, []domain.Vhtlc, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, nil, err
	}

	// Get VHTLCs based on filter
	var vhtlcList []domain.Vhtlc
	vhtlcRepo := s.dbSvc.VHTLC()

	if vhtlc_id != "" {
		vhtlc, err := vhtlcRepo.Get(ctx, vhtlc_id)
		if err != nil {
			return nil, nil, err
		}
		vhtlcList = []domain.Vhtlc{*vhtlc}
	} else {
		var err error
		vhtlcList, err = vhtlcRepo.GetAll(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	vhtlcOpts := make([]vhtlc.Opts, 0, len(vhtlcList))
	for _, v := range vhtlcList {
		vhtlcOpts = append(vhtlcOpts, v.Opts)
	}

	vtxos, err := swapHandler.GetVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return nil, nil, err
	}

	return vtxos, vhtlcList, nil
}

func (s *Service) ClaimSwapVHTLC(ctx context.Context, preimage []byte, vhtlc_id string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	return swapHandler.ClaimVHTLC(ctx, preimage, vhtlc.Opts)
}

func (s *Service) RefundSwapVHTLC(ctx context.Context, swapId, vhtlc_id string, withReceiver bool) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	return swapHandler.RefundSwap(ctx, swapId, withReceiver, vhtlc.Opts)
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

func (s *Service) GetBalanceLN(ctx context.Context) (balance uint64, err error) {
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

	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return "", fmt.Errorf("failed to generate preimage: %w", err)
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	var wg sync.WaitGroup
	wg.Add(1)

	postProcess := func(swapData swap.Swap) error {
		defer wg.Done()

		if swapData.Status != swap.SwapSuccess {
			return nil
		}

		vHTLC := domain.NewVhtlc(*swapData.Opts)

		err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:         swapData.Id,
			Type:       domain.SwapRegular,
			Amount:     swapData.Amount,
			From:       boltz.CurrencyBtc,
			To:         boltz.CurrencyArk,
			Vhtlc:      vHTLC,
			Timestamp:  swapData.Timestamp,
			RedeemTxId: swapData.RedeemTxid,
			Status:     domain.SwapStatus(swapData.Status),
		})

		return err

	}

	swapDetails, err := swapHandler.GetInvoice(ctx, amount, postProcess)

	// Pay the invoice to reveal the preimage
	if _, err := s.payInvoiceLN(ctx, swapDetails.Invoice); err != nil {
		return "", fmt.Errorf("failed to pay invoice: %v", err)
	}

	wg.Wait()
	swap, err := s.dbSvc.Swap().Get(ctx, swapDetails.Id)
	if err != nil {
		return "", err
	}

	return swap.RedeemTxId, err
}

// ark -> ln (submarine swap)
func (s *Service) IncreaseOutboundCapacity(ctx context.Context, amount uint64) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	boltzApi := s.boltzSvc

	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapHandler, swapData.Id, *swapData.Opts)
		return err
	}

	// Get invoice from the connected LN service
	invoice, preimageHashStr, err := s.getInvoiceLN(ctx, amount, "increase outbound capacity", "")
	if err != nil {
		return SwapResponse{}, fmt.Errorf("failed to create invoice: %w", err)
	}

	_, err = hex.DecodeString(preimageHashStr)
	if err != nil {
		return SwapResponse{}, fmt.Errorf("failed to decode preimage hash: %v", err)
	}

	swapDetails, err := swapHandler.PayInvoice(ctx, invoice, unilateralRefund)

	if err != nil {
		return SwapResponse{}, err
	}

	swapStatus := domain.SwapStatus(swapDetails.Status)
	vHTLC := domain.NewVhtlc(*swapDetails.Opts)

	go func() {
		dbErr := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:          swapDetails.Id,
			Type:        domain.SwapRegular,
			Amount:      swapDetails.Amount,
			From:        boltz.CurrencyArk,
			Timestamp:   swapDetails.Timestamp,
			To:          boltz.CurrencyBtc,
			Vhtlc:       vHTLC,
			FundingTxId: swapDetails.TxId,
			Status:      swapStatus,
		})

		if dbErr != nil {
			log.WithError(dbErr).Error("failed to add swap to db")
			return
		}

	}()

	return SwapResponse{TxId: swapDetails.TxId, SwapStatus: swapStatus, Invoice: swapDetails.Invoice}, err
}

func (s *Service) SubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		pkScript, err := offchainAddressPkScript(addr)
		if err != nil {
			return fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}

		scripts = append(scripts, pkScript)
	}

	return s.externalSubscription.subscribe(ctx, scripts)
}

func (s *Service) UnsubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		pkScript, err := offchainAddressPkScript(addr)
		if err != nil {
			return fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}
		scripts = append(scripts, pkScript)
	}

	return s.externalSubscription.unsubscribe(ctx, scripts)
}

func (s *Service) GetVtxoNotifications(ctx context.Context) <-chan Notification {
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

	return s.dbSvc.VtxoRollover().AddTarget(ctx, target)
}

func (s *Service) UnwatchAddress(ctx context.Context, address string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	if address == "" {
		return fmt.Errorf("missing address")
	}

	return s.dbSvc.VtxoRollover().DeleteTarget(ctx, address)
}

func (s *Service) ListWatchedAddresses(ctx context.Context) ([]domain.VtxoRolloverTarget, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	return s.dbSvc.VtxoRollover().GetAllTargets(ctx)
}

func (s *Service) IsLocked(ctx context.Context) bool {
	if s.ArkClient == nil {
		return false
	}

	return s.ArkClient.IsLocked(ctx)
}

func (s *Service) GetInvoice(ctx context.Context, amount uint64) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	postProcess := func(swapData swap.Swap) error {
		if swapData.Status != swap.SwapSuccess {
			return nil
		}

		vHTLC := domain.NewVhtlc(*swapData.Opts)

		err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:         swapData.Id,
			Type:       domain.SwapPayment,
			Amount:     swapData.Amount,
			From:       boltz.CurrencyBtc,
			To:         boltz.CurrencyArk,
			Vhtlc:      vHTLC,
			Timestamp:  swapData.Timestamp,
			RedeemTxId: swapData.RedeemTxid,
			Status:     domain.SwapStatus(swapData.Status),
		})

		return err

	}

	swapDetails, err := swapHandler.GetInvoice(ctx, amount, postProcess)
	if err != nil {
		if strings.Contains(err.Error(), "out of limits") {
			return SwapResponse{}, nil
		}

		return SwapResponse{}, err
	}

	return SwapResponse{Invoice: swapDetails.Invoice, SwapStatus: domain.SwapPending}, err

}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	boltzApi := s.boltzSvc

	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapHandler, swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := swapHandler.PayInvoice(ctx, invoice, unilateralRefund)

	if err != nil {
		return SwapResponse{}, err
	}

	swapStatus := domain.SwapStatus(swapDetails.Status)
	vHTLC := domain.NewVhtlc(*swapDetails.Opts)

	go func() {
		dbErr := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:          swapDetails.Id,
			Type:        domain.SwapPayment,
			Amount:      swapDetails.Amount,
			From:        boltz.CurrencyArk,
			Timestamp:   swapDetails.Timestamp,
			To:          boltz.CurrencyBtc,
			Vhtlc:       vHTLC,
			FundingTxId: swapDetails.TxId,
			Status:      swapStatus,
		})

		if dbErr != nil {
			log.WithError(dbErr).Error("failed to add swap to db")
			return
		}

	}()

	return SwapResponse{TxId: swapDetails.TxId, SwapStatus: swapStatus, Invoice: swapDetails.Invoice}, err
}

func (s *Service) PayOffer(ctx context.Context, offer string) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	configData, err := s.GetConfigData(ctx)
	if err != nil {
		return SwapResponse{}, fmt.Errorf("failed to get config data: %v", err)
	}

	boltzApi := s.boltzSvc
	var lightningUrl string

	if configData.Network.Name == arklib.BitcoinRegTest.Name {
		boltzUrl, err := url.Parse(s.boltzSvc.URL)
		if err != nil {
			return SwapResponse{}, err
		}
		host := boltzUrl.Hostname()
		boltzUrl.Host = fmt.Sprintf("%s:%d", host, 9005)
		lightningUrl = boltzUrl.String()
	}

	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapHandler, swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := swapHandler.PayOffer(ctx, offer, lightningUrl, unilateralRefund)

	if err != nil {
		return SwapResponse{}, err
	}

	swapStatus := domain.SwapStatus(swapDetails.Status)
	vHTLC := domain.NewVhtlc(*swapDetails.Opts)

	go func() {
		dbErr := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:          swapDetails.Id,
			Type:        domain.SwapPayment,
			Amount:      swapDetails.Amount,
			From:        boltz.CurrencyArk,
			To:          boltz.CurrencyBtc,
			Vhtlc:       vHTLC,
			FundingTxId: swapDetails.TxId,
			Timestamp:   swapDetails.Timestamp,
			Status:      swapStatus,
		})

		if dbErr != nil {
			log.WithError(dbErr).Error("failed to add swap to db")
			return
		}

	}()

	return SwapResponse{TxId: swapDetails.TxId, SwapStatus: swapStatus, Invoice: swapDetails.Invoice}, err
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

func (s *Service) boltzRefundSwap(swapId, refundTx string) (string, error) {
	tx, err := s.boltzSvc.RefundSubmarine(swapId, boltz.RefundSwapRequest{
		Transaction: refundTx,
	})
	if err != nil {
		return "", err
	}

	return tx.Transaction, nil
}

func (s *Service) computeNextExpiry(ctx context.Context, data *types.Config) (*time.Time, error) {
	spendableVtxos, _, err := s.ListVtxos(ctx)
	if err != nil {
		return nil, err
	}

	var expiry *time.Time

	if len(spendableVtxos) > 0 {
		for _, vtxo := range spendableVtxos[:] {
			if vtxo.ExpiresAt.Before(time.Now()) {
				continue
			}

			if expiry == nil || vtxo.ExpiresAt.Before(*expiry) {
				expiry = &vtxo.ExpiresAt
			}
		}

	}

	txs, err := s.GetTransactionHistory(ctx)
	if err != nil {
		return nil, err
	}

	// check for unsettled boarding UTXOs
	for _, tx := range txs {
		if len(tx.BoardingTxid) > 0 && !tx.Settled {
			// TODO replace by boardingExitDelay https://github.com/ark-network/ark/pull/501
			boardingExpiry := tx.CreatedAt.Add(time.Duration(data.UnilateralExitDelay.Seconds()*2) * time.Second)
			if boardingExpiry.Before(time.Now()) {
				continue
			}

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
	eventsCh := s.GetUtxoEventChannel(ctx)
	boardingScript, err := onchainAddressPkScript(address, cfg.Network)
	if err != nil {
		log.WithError(err).Error("failed to get output script")
		return
	}

	for {
		select {
		case <-s.stopBoardingEventListener:
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			if event.Type == 0 && len(event.Utxos) == 0 {
				continue
			}

			filteredUtxos := make([]types.Utxo, 0, len(event.Utxos))
			for _, utxo := range event.Utxos {
				if utxo.Spent || !utxo.IsConfirmed() {
					continue
				}

				if utxo.Script == boardingScript {
					filteredUtxos = append(filteredUtxos, utxo)
				}
			}

			// if expiry is before the next scheduled settlement, we need to schedule a new one
			if len(filteredUtxos) > 0 {
				log.Infof("boarding event detected: %d new confirmed utxos", len(filteredUtxos))
				nextScheduledSettlement := s.WhenNextSettlement(ctx)

				needSchedule := false

				for _, utxo := range filteredUtxos {
					if nextScheduledSettlement.IsZero() || utxo.SpendableAt.Before(nextScheduledSettlement) {
						nextScheduledSettlement = utxo.SpendableAt
						needSchedule = true
					}
				}

				if needSchedule {
					if err := s.scheduleNextSettlement(nextScheduledSettlement, cfg); err != nil {
						log.WithError(err).Info("schedule next claim failed")
					}
				}
			}
		}
	}
}

// handleAddressEventChannel is used to forward address events to the notifications channel
func (s *Service) handleAddressEventChannel(config *types.Config) func(event *indexer.ScriptEvent) {
	return func(event *indexer.ScriptEvent) {
		if event == nil {
			log.Warn("Received nil event from event channel")
			return
		}

		if event.Err != nil {
			log.WithError(event.Err).Error("AddressEvent subscription error")
			return
		}

		log.Infof("received address event(%d spent vtxos, %d new vtxos)", len(event.SpentVtxos), len(event.NewVtxos))

		// convert scripts to addresses
		addresses := make([]string, 0, len(event.Scripts))
		for _, script := range event.Scripts {
			decodedPubKey, err := hex.DecodeString(script)
			if err != nil {
				log.WithError(err).Errorf("failed to decode script %s", script)
				continue
			}
			vtxoTapPubkey, err := schnorr.ParsePubKey(decodedPubKey[2:])
			if err != nil {
				log.WithError(err).Errorf("failed to parse pubkey %s", script)
				continue
			}

			vtxoAddress := arklib.Address{
				VtxoTapKey: vtxoTapPubkey,
				Signer:     config.SignerPubKey,
				HRP:        config.Network.Addr,
			}

			encodedAddress, err := vtxoAddress.EncodeV0()
			if err != nil {
				log.WithError(err).Errorf("failed to encode address %s", script)
				continue
			}
			addresses = append(addresses, encodedAddress)

		}

		// non-blocking forward to notifications channel
		go func(evt *indexer.ScriptEvent) {
			s.notifications <- Notification{
				Addrs:       addresses,
				NewVtxos:    event.NewVtxos,
				SpentVtxos:  event.SpentVtxos,
				Checkpoints: event.CheckpointTxs,
				TxData:      indexer.TxData{Tx: event.Tx, Txid: event.Txid},
			}
		}(event)
	}
}

// handleInternalAddressEventChannel is used to handle address events from the internal address event channel
// it is used to schedule next settlement when a VTXO is spent or created
func (s *Service) handleInternalAddressEventChannel(event *indexer.ScriptEvent) {
	if event.Err != nil {
		log.WithError(event.Err).Error("AddressEvent subscription error")
		return
	}

	ctx := context.Background()

	data, err := s.GetConfigData(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get config data")
		return
	}

	log.Infof("received internal address event (%d spent vtxos, %d new vtxos)", len(event.SpentVtxos), len(event.NewVtxos))

	// if some vtxos were spent, schedule a settlement to soonest expiry among new vtxos / boarding UTXOs set
	if len(event.SpentVtxos) > 0 {
		nextExpiry, err := s.computeNextExpiry(ctx, data)
		if err != nil {
			log.WithError(err).Error("failed to compute next expiry")
			return
		}

		if nextExpiry != nil {
			if err := s.scheduleNextSettlement(*nextExpiry, data); err != nil {
				log.WithError(err).Info("schedule next claim failed")
			}
		}

		return
	}

	// if some vtxos were created, schedule a settlement to the soonest expiry among new vtxos
	if len(event.NewVtxos) > 0 {
		nextScheduledSettlement := s.WhenNextSettlement(ctx)

		needSchedule := false

		for _, vtxo := range event.NewVtxos {
			log.Infof("new vtxo: %s, expires at: %s", vtxo.Txid, vtxo.ExpiresAt.Format(time.RFC3339))
			if nextScheduledSettlement.IsZero() || vtxo.ExpiresAt.Before(nextScheduledSettlement) {
				nextScheduledSettlement = vtxo.ExpiresAt
				needSchedule = true
			}
		}

		if needSchedule {
			if err := s.scheduleNextSettlement(nextScheduledSettlement, data); err != nil {
				log.WithError(err).Info("schedule next claim failed")
			}
		}
	}
}

func (s *Service) getInvoiceLN(ctx context.Context, amount uint64, memo, preimage string) (string, string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.GetInvoice(ctx, amount, memo, preimage)
}

func (s *Service) payInvoiceLN(ctx context.Context, invoice string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.PayInvoice(ctx, invoice)
}

func (s *Service) scheduleSwapRefund(swapHandler *swap.SwapHandler, swapId string, opts vhtlc.Opts) (err error) {
	unilateral := func() {
		vtxos, err := swapHandler.GetVHTLCFunds(context.Background(), []vhtlc.Opts{opts})
		if err != nil {
			log.WithError(err).Error("failed to check vhtlc status")
			return
		}
		if len(vtxos) == 0 {
			log.WithError(err).Errorf("vhtlc %s not found", opts.PreimageHash)
			return
		}

		if vtxos[0].Spent {
			log.Infof("vhtlc %s already spent", opts.PreimageHash)

			swapData := domain.Swap{
				Id:     swapId,
				Status: domain.SwapSuccess,
			}

			if err := s.dbSvc.Swap().Update(context.Background(), swapData); err != nil {
				log.WithError(err).Error("failed to add swap data to db")
			}
			return
		}

		txid, err := swapHandler.RefundSwap(context.Background(), swapId, false, opts)
		if err != nil {
			log.WithError(err).Error("failed to refund vhtlc")
			return
		}

		swapData := domain.Swap{
			Id:         swapId,
			Status:     domain.SwapFailed,
			RedeemTxId: txid,
		}

		if err := s.dbSvc.Swap().Update(context.Background(), swapData); err != nil {
			log.WithError(err).Error("failed to add payment data to db")
		}

		log.Infof("vhtlc refunded %s", txid)
	}

	refundLT := opts.RefundLocktime

	if refundLT.IsSeconds() {
		err = s.schedulerSvc.ScheduleRefundAtTime(time.Unix(int64(refundLT), 0), unilateral)
	} else {
		log.Infof("scheduling vhtlc refund at height %d", refundLT)
		err = s.schedulerSvc.ScheduleRefundAtHeight(uint32(refundLT), unilateral)
	}

	return err
}

func (s *Service) resumePendingSwapRefunds(ctx context.Context) {
	swaps, err := s.dbSvc.Swap().GetAll(ctx)
	if err != nil {
		log.WithError(err).Error("failed to load swaps while rescheduling refunds")
		return
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.IndexerClient, boltzApi, s.publicKey, s.swapTimeout)

	for _, swap := range swaps {

		if swap.Status == domain.SwapFailed && swap.RedeemTxId == "" {
			if err := s.scheduleSwapRefund(swapHandler, swap.Id, swap.Vhtlc.Opts); err != nil {
				log.WithError(err).WithField("swap_id", swap.Id).Warn("failed to reschedule refund task")
			}
		}

	}
}

func parsePubkey(pubkey string) (*btcec.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	dec, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	pk, err := btcec.ParsePubKey(dec)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}

func (s *Service) GetSwapHistory(ctx context.Context) ([]domain.Swap, error) {
	all, err := s.dbSvc.Swap().GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap history: %w", err)
	}
	if len(all) == 0 {
		return all, nil
	}
	// sort swaps by timestamp descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp > all[j].Timestamp
	})
	return all, nil
}

// verifyInputSignatures checks that all inputs have a signature for the given pubkey
// and the signature is correct for the given tapscript leaf
func verifyInputSignatures(tx *psbt.Packet, pubkey *btcec.PublicKey, tapLeaves map[int]txscript.TapLeaf) error {
	xOnlyPubkey := schnorr.SerializePubKey(pubkey)

	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	sigsToVerify := make(map[int]*psbt.TaprootScriptSpendSig)

	for inputIndex, input := range tx.Inputs {
		// collect previous outputs
		if input.WitnessUtxo == nil {
			return fmt.Errorf("input %d has no witness utxo, cannot verify signature", inputIndex)
		}

		outpoint := tx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint
		prevouts[outpoint] = input.WitnessUtxo

		tapLeaf, ok := tapLeaves[inputIndex]
		if !ok {
			return fmt.Errorf("input %d has no tapscript leaf, cannot verify signature", inputIndex)
		}

		tapLeafHash := tapLeaf.TapHash()

		// check if pubkey has a tapscript sig
		hasSig := false
		for _, sig := range input.TaprootScriptSpendSig {
			if bytes.Equal(sig.XOnlyPubKey, xOnlyPubkey) && bytes.Equal(sig.LeafHash, tapLeafHash[:]) {
				hasSig = true
				sigsToVerify[inputIndex] = sig
				break
			}
		}

		if !hasSig {
			return fmt.Errorf("input %d has no signature for pubkey %x", inputIndex, xOnlyPubkey)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txSigHashes := txscript.NewTxSigHashes(tx.UnsignedTx, prevoutFetcher)

	for inputIndex, sig := range sigsToVerify {
		msgHash, err := txscript.CalcTapscriptSignaturehash(
			txSigHashes,
			sig.SigHash,
			tx.UnsignedTx,
			inputIndex,
			prevoutFetcher,
			tapLeaves[inputIndex],
		)
		if err != nil {
			return fmt.Errorf("failed to calculate tapscript signature hash: %w", err)
		}

		signature, err := schnorr.ParseSignature(sig.Signature)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		if !signature.Verify(msgHash, pubkey) {
			return fmt.Errorf("input %d: invalid signature", inputIndex)
		}
	}

	return nil
}

func offchainAddressPkScript(addr string) (string, error) {
	decodedAddress, err := arklib.DecodeAddressV0(addr)
	if err != nil {
		return "", fmt.Errorf("failed to decode address %s: %w", addr, err)
	}

	p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse address to p2tr script: %w", err)
	}
	return hex.EncodeToString(p2trScript), nil
}

func onchainAddressPkScript(addr string, network arklib.Network) (string, error) {
	btcAddress, err := btcutil.DecodeAddress(addr, toBitcoinNetwork(network))
	if err != nil {
		return "", fmt.Errorf("failed to decode address %s: %w", addr, err)
	}

	script, err := txscript.PayToAddrScript(btcAddress)
	if err != nil {
		return "", fmt.Errorf("failed to parse address to p2tr script: %w", err)
	}
	return hex.EncodeToString(script), nil
}

type internalScriptsStore string

func (s internalScriptsStore) Get(ctx context.Context) ([]string, error) {
	return []string{string(s)}, nil
}

func (s internalScriptsStore) Add(ctx context.Context, scripts []string) (int, error) {
	return 0, fmt.Errorf("cannot add scripts to internal subscription")
}

func (s internalScriptsStore) Delete(ctx context.Context, scripts []string) (int, error) {
	return 0, fmt.Errorf("cannot delete scripts from internal subscription")
}

func toBitcoinNetwork(net arklib.Network) *chaincfg.Params {
	switch net.Name {
	case arklib.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case arklib.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	//case arklib.BitcoinTestNet4.Name: //TODO uncomment once supported
	//	return chaincfg.TestNet4Params
	case arklib.BitcoinSigNet.Name:
		return &chaincfg.SigNetParams
	case arklib.BitcoinMutinyNet.Name:
		return &arklib.MutinyNetSigNetParams
	case arklib.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
}

func deriveTimelock(timelock uint32) arklib.RelativeLocktime {
	if timelock >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: timelock}
	}

	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: timelock}
}
