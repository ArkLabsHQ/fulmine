package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/cln"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/lnd"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
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
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/ccoveille/go-safecast"
	"github.com/lightningnetwork/lnd/input"
	log "github.com/sirupsen/logrus"
)

const (
	WalletInit                                  = "init"
	WalletUnlock                                = "unlock"
	WalletReset                                 = "reset"
	defaultUnilateralClaimDelay                 = 512
	defaultUnilateralRefundDelay                = 1024
	defaultUnilateralRefundWithoutReceiverDelay = 224
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
	BuildInfo BuildInfo

	arksdk.ArkClient
	storeCfg      store.Config
	storeRepo     types.Store
	dbSvc         ports.RepoManager
	grpcClient    client.TransportClient
	indexerClient indexer.Indexer
	schedulerSvc  ports.SchedulerService
	lnSvc         ports.LnService
	boltzSvc      *boltz.Api

	publicKey *btcec.PublicKey

	esploraUrl string
	boltzUrl   string
	boltzWSUrl string

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

func NewService(
	buildInfo BuildInfo,
	storeCfg store.Config,
	storeSvc types.Store,
	dbSvc ports.RepoManager,
	schedulerSvc ports.SchedulerService,
	esploraUrl, boltzUrl, boltzWSUrl string,
	connectionOpts *domain.LnConnectionOpts,
) (*Service, error) {
	if arkClient, err := arksdk.LoadArkClient(storeSvc); err == nil {
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
			indexerClient:             indexerClient,
			schedulerSvc:              schedulerSvc,
			publicKey:                 nil,
			isReady:                   true,
			notifications:             make(chan Notification),
			stopBoardingEventListener: make(chan struct{}),
			esploraUrl:                data.ExplorerURL,
			boltzUrl:                  boltzUrl,
			boltzWSUrl:                boltzWSUrl,
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

	arkClient, err := arksdk.NewArkClient(storeSvc)
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
	s.indexerClient = indexerClient
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

	invoice, err = s.GetInvoice(ctx, sats)
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
	return s.indexerClient.GetCommitmentTx(ctx, roundId)
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

	// if market hour is set, schedule the task at the best market hour = market hour closer to `at` timestamp

	marketHourStartTime := time.Unix(data.MarketHourStartTime, 0)
	if !marketHourStartTime.IsZero() && data.MarketHourPeriod > 0 && at.After(marketHourStartTime) {
		cycles := math.Floor(at.Sub(marketHourStartTime).Seconds() / float64(data.MarketHourPeriod))
		at = marketHourStartTime.Add(time.Duration(cycles) * time.Duration(data.MarketHourPeriod) * time.Second)
	}

	roundInterval := time.Duration(data.RoundInterval) * time.Second
	at = at.Add(-2 * roundInterval) // schedule 2 rounds before the expiry

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

func (s *Service) GetVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *arklib.AbsoluteLocktime,
	unilateralClaimDelayParam *arklib.RelativeLocktime,
	unilateralRefundDelayParam *arklib.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *arklib.RelativeLocktime,
) (string, *vhtlc.VHTLCScript, *vhtlc.Opts, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", nil, nil, err
	}

	// check if preimage hash already exists in DB
	preimageHashStr := hex.EncodeToString(preimageHash)
	if _, err := s.dbSvc.VHTLC().Get(ctx, preimageHashStr); err == nil {
		return "", nil, nil, fmt.Errorf("vHTLC with preimage hash %s already exists", preimageHashStr)
	}

	addr, vhtlcScript, opts, err := s.getVHTLC(
		ctx, receiverPubkey, senderPubkey, preimageHash,
		refundLocktimeParam, unilateralClaimDelayParam, unilateralRefundDelayParam,
		unilateralRefundWithoutReceiverDelayParam,
	)
	if err != nil {
		return "", nil, nil, err
	}

	go func() {
		if err := s.dbSvc.VHTLC().Add(context.Background(), *opts); err != nil {
			log.WithError(err).Error("failed to add vhtlc")
			return
		}

		log.Debugf("added new vhtlc %x", preimageHash)
	}()

	return addr, vhtlcScript, opts, nil
}

func (s *Service) ListVHTLC(ctx context.Context, preimageHashFilter string) ([]types.Vtxo, []vhtlc.Opts, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, nil, err
	}

	// Get VHTLC options based on filter
	var vhtlcOpts []vhtlc.Opts
	vhtlcRepo := s.dbSvc.VHTLC()

	if preimageHashFilter != "" {
		opt, err := vhtlcRepo.Get(ctx, preimageHashFilter)
		if err != nil {
			return nil, nil, err
		}
		vhtlcOpts = []vhtlc.Opts{*opt}
	} else {
		var err error
		vhtlcOpts, err = vhtlcRepo.GetAll(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	vtxos, err := s.getVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return nil, nil, err
	}

	return vtxos, vhtlcOpts, nil
}

func (s *Service) ClaimVHTLC(ctx context.Context, preimage []byte) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimage))
	vhtlcOpts, err := s.dbSvc.VHTLC().Get(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	return s.claimVHTLC(ctx, preimage, *vhtlcOpts)
}

func (s *Service) RefundVHTLC(ctx context.Context, swapId, preimageHash string, withReceiver bool) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlcOpts, err := s.dbSvc.VHTLC().Get(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	return s.refundVHTLC(ctx, swapId, withReceiver, *vhtlcOpts)
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

	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return "", fmt.Errorf("failed to generate preimage: %w", err)
	}

	return s.reverseSwap(ctx, amount, preimage, s.publicKey.SerializeCompressed())
}

// ark -> ln (submarine swap)
func (s *Service) IncreaseOutboundCapacity(ctx context.Context, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	return s.submarineSwap(ctx, amount)
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

func (s *Service) GetInvoice(ctx context.Context, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return "", fmt.Errorf("failed to generate preimage: %w", err)
	}

	return s.reverseSwapWithPreimage(ctx, amount, preimage, s.publicKey.SerializeCompressed())
}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	_, _, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", err
	}
	pubkey, _ := hex.DecodeString(pk)

	return s.submarineSwapWithInvoice(ctx, invoice, pubkey)
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

// swap takes care of interacting with the Boltz server to make a submarine swap.
// The function can be used by passing either an amount or an invoice. The args are mutually exclusive.
// When passing an amount, the invoice is generated by us, otherwise it means its generated by
// somebody else. In any case, we fund the VHTLC and make sure that it succeeds before returning,
// otherwise the VHTLC is refunded if necessary.
func (s *Service) submarineSwap(ctx context.Context, amount uint64) (string, error) {
	// Get our pubkey
	_, _, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get pubkey: %v", err)
	}
	myPubkey, _ := hex.DecodeString(pk)

	var preimageHash []byte

	// Get invoice from the connected LN service
	invoice, preimageHashStr, err := s.getInvoiceLN(ctx, amount, "increase outbound capacity", "")
	if err != nil {
		return "", fmt.Errorf("failed to create invoice: %w", err)
	}
	// nolint
	preimageHash, _ = hex.DecodeString(preimageHashStr)

	// Create the swap
	swap, err := s.boltzSvc.CreateSwap(boltz.CreateSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		Invoice:         invoice,
		RefundPublicKey: hex.EncodeToString(myPubkey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make submarine swap: %v", err)
	}

	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid claim pubkey: %v", err)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	vhtlcAddress, _, opts, err := s.getVHTLC(
		ctx,
		receiverPubkey,
		nil,
		preimageHash,
		&refundLocktime,
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// Fund the VHTLC
	receivers := []types.Receiver{{To: swap.Address, Amount: swap.ExpectedAmount}}
	txid, err := s.SendOffChain(ctx, false, receivers)
	if err != nil {
		return "", fmt.Errorf("failed to pay to vHTLC address: %v", err)
	}

	// Workaround to connect ws endpoint on a different port for regtest
	wsClient := s.boltzSvc
	if s.boltzSvc.URL == boltzURLByNetwork[arklib.BitcoinRegTest.Name] {
		wsClient = &boltz.Api{WSURL: "http://localhost:9004"}
	}

	ws := wsClient.NewWebsocket()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = ws.Connect()
	for err != nil {
		log.WithError(err).Warn("failed to connect to boltz websocket")
		time.Sleep(time.Second)
		log.Debug("reconnecting...")
		err = ws.Connect()
		if ctx.Err() != nil {
			return "", fmt.Errorf("timeout while connecting to websocket: %v", ctx.Err())
		}
	}

	err = ws.Subscribe([]string{swap.Id})
	for err != nil {
		log.WithError(err).Warn("failed to subscribe for swap events")
		time.Sleep(time.Second)
		log.Debug("retrying...")
		err = ws.Subscribe([]string{swap.Id})
	}

	for update := range ws.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		switch parsedStatus {
		case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
			// Refund the VHTLC if the swap fails
			withReceiver := true
			refundTxid, err := s.RefundVHTLC(
				context.Background(), swap.Id, hex.EncodeToString(preimageHash), withReceiver,
			)
			if err != nil {
				return "", fmt.Errorf("failed to refund vHTLC: %s", err)
			}

			go func() {
				if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
					Id:          swap.Id,
					Amount:      amount,
					Timestamp:   time.Now().Unix(),
					Status:      domain.SwapFailed,
					Invoice:     invoice,
					FundingTxId: txid,
					RedeemTxId:  refundTxid,
					To:          boltz.CurrencyBtc,
					From:        boltz.CurrencyArk,
					VhtlcOpts:   *opts,
				}); err != nil {
					log.WithError(err).Fatal("failed to store swap")
				}
				log.Debugf("added new refunded swap %s", swap.Id)
			}()

			return "", fmt.Errorf("something went wrong, the vhtlc was refunded %s", txid)
		case boltz.TransactionClaimed, boltz.InvoiceSet:
			// Nothing left to do, return the VHTLC funding txid
			go func() {
				if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
					Id:          swap.Id,
					Amount:      amount,
					Timestamp:   time.Now().Unix(),
					Status:      domain.SwapSuccess,
					Invoice:     invoice,
					FundingTxId: txid,
					To:          boltz.CurrencyBtc,
					From:        boltz.CurrencyArk,
					VhtlcOpts:   *opts,
				}); err != nil {
					log.WithError(err).Fatal("failed to store swap")
				}
				log.Debugf("added new swap %s", swap.Id)
			}()
			return txid, nil
		}
	}

	return "", fmt.Errorf("something went wrong")
}

func (s *Service) submarineSwapWithInvoice(ctx context.Context, invoice string, pubkey []byte) (string, error) {
	if len(invoice) == 0 {
		return "", fmt.Errorf("invoice must not be empty")
	}

	_, preimageHash, err := utils.DecodeInvoice(invoice)
	if err != nil {
		return "", fmt.Errorf("failed to decode invoice: %v", err)
	}

	// Create the swap
	swap, err := s.boltzSvc.CreateSwap(boltz.CreateSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		Invoice:         invoice,
		RefundPublicKey: hex.EncodeToString(pubkey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make submarine swap: %v", err)
	}

	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid claim pubkey: %v", err)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	vhtlcAddress, _, _, err := s.getVHTLC(
		ctx,
		receiverPubkey,
		nil,
		preimageHash,
		&refundLocktime,
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// Fund the VHTLC
	receivers := []types.Receiver{{To: swap.Address, Amount: swap.ExpectedAmount}}
	txid, err := s.SendOffChain(ctx, false, receivers)
	if err != nil {
		return "", fmt.Errorf("failed to pay to vHTLC address: %v", err)
	}

	// Workaround to connect ws endpoint on a different port for regtest
	wsClient := s.boltzSvc
	if s.boltzSvc.URL == boltzURLByNetwork[arklib.BitcoinRegTest.Name] {
		wsClient = &boltz.Api{WSURL: "http://localhost:9004"}
	}

	ws := wsClient.NewWebsocket()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = ws.Connect()
	for err != nil {
		log.WithError(err).Warn("failed to connect to boltz websocket")
		time.Sleep(time.Second)
		log.Debug("reconnecting...")
		err = ws.Connect()
		if ctx.Err() != nil {
			return "", fmt.Errorf("timeout while connecting to websocket: %v", ctx.Err())
		}
	}

	err = ws.Subscribe([]string{swap.Id})
	for err != nil {
		log.WithError(err).Warn("failed to subscribe for swap events")
		time.Sleep(time.Second)
		log.Debug("retrying...")
		err = ws.Subscribe([]string{swap.Id})
	}

	for update := range ws.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		switch parsedStatus {
		case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
			// Refund the VHTLC if the swap fails
			withReceiver := true
			txid, err := s.RefundVHTLC(
				context.Background(), swap.Id, hex.EncodeToString(preimageHash), withReceiver,
			)
			if err != nil {
				return "", fmt.Errorf("failed to refund vHTLC: %s", err)
			}

			return txid, fmt.Errorf("something went wrong, the vhtlc was refunded %s", txid)
		case boltz.TransactionClaimed, boltz.InvoiceSet:
			// Nothing left to do, return the VHTLC funding txid
			return txid, nil
		}
	}

	return "", fmt.Errorf("something went wrong")
}

// reverseSwap takes care of interacting with the Boltz server to make a reverse submarine swap.
// Passing a preimage to this function means that the invoice generated by Boltz is expected
// to be paid by somebody else, and therefore the swap status is watched before claiming the funds
// locked in the VHTLC.
// When the preimage is empty, the invoice returned by Boltz is expected to be paid by us, the preimage
// is revealed and the funds locked in the VHTLC can be claimed without checking the swap status.
func (s *Service) reverseSwap(ctx context.Context, amount uint64, preimage, myPubkey []byte) (string, error) {
	// make swap
	buf := sha256.Sum256(preimage)

	swap, err := s.boltzSvc.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           boltz.CurrencyBtc,
		To:             boltz.CurrencyArk,
		InvoiceAmount:  amount,
		ClaimPublicKey: hex.EncodeToString(myPubkey),
		PreimageHash:   hex.EncodeToString(buf[:]),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make reverse submarine swap: %v", err)
	}

	// verify vHTLC
	senderPubkey, err := parsePubkey(swap.RefundPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid refund pubkey: %v", err)
	}

	// verify preimage hash and invoice amount
	invoiceAmount, gotPreimageHash, err := utils.DecodeInvoice(swap.Invoice)
	if err != nil {
		return "", fmt.Errorf("failed to decode invoice: %v", err)
	}

	if invoiceAmount != amount {
		return "", fmt.Errorf("invalid invoice amount: expected %d, got %d", amount, invoiceAmount)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	vhtlcAddress, _, opts, err := s.getVHTLC(
		ctx,
		nil,
		senderPubkey,
		gotPreimageHash,
		&refundLocktime,
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// Pay the invoice to reveal the preimage
	if _, err := s.payInvoiceLN(ctx, swap.Invoice); err != nil {
		return "", fmt.Errorf("failed to pay invoice: %v", err)
	}

	txid, err := s.waitAndClaimVHTLC(ctx, swap.Id, preimage, opts)
	if err != nil {
		return "", fmt.Errorf("failed to claim vHTLC: %v", err)
	}

	go func() {
		if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:         swap.Id,
			Amount:     amount,
			Timestamp:  time.Now().Unix(),
			Invoice:    swap.Invoice,
			To:         boltz.CurrencyArk,
			From:       boltz.CurrencyBtc,
			Status:     domain.SwapSuccess,
			VhtlcOpts:  *opts,
			RedeemTxId: txid,
		}); err != nil {
			log.WithError(err).Fatal("failed to store swap")
		}
		log.Debugf("added new swap %s", swap.Id)
	}()

	return txid, nil

}

func (s *Service) reverseSwapWithPreimage(ctx context.Context, amount uint64, preimage, myPubkey []byte) (string, error) {
	var preimageHash []byte
	buf := sha256.Sum256(preimage)
	preimageHash = input.Ripemd160H(buf[:])

	// make swap
	swap, err := s.boltzSvc.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           boltz.CurrencyBtc,
		To:             boltz.CurrencyArk,
		InvoiceAmount:  amount,
		ClaimPublicKey: hex.EncodeToString(myPubkey),
		PreimageHash:   hex.EncodeToString(buf[:]),
	})
	if err != nil {
		if strings.Contains(err.Error(), "out of limits") {
			return "", nil
		}
		return "", fmt.Errorf("failed to make reverse submarine swap: %v", err)
	}

	// verify vHTLC
	senderPubkey, err := parsePubkey(swap.RefundPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid refund pubkey: %v", err)
	}

	// verify preimage hash and invoice amount
	invoiceAmount, gotPreimageHash, err := utils.DecodeInvoice(swap.Invoice)
	if err != nil {
		return "", fmt.Errorf("failed to decode invoice: %v", err)
	}

	if !bytes.Equal(preimageHash, gotPreimageHash) {
		return "", fmt.Errorf("invalid preimage hash: expected %x, got %x", preimageHash, gotPreimageHash)
	}
	if invoiceAmount != amount {
		return "", fmt.Errorf("invalid invoice amount: expected %d, got %d", amount, invoiceAmount)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	vhtlcAddress, _, vhtlcOpts, err := s.getVHTLC(
		ctx,
		nil,
		senderPubkey,
		gotPreimageHash,
		&refundLocktime,
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	go func() {
		if _, err := s.waitAndClaimVHTLC(
			context.Background(), swap.Id, preimage, vhtlcOpts,
		); err != nil {
			log.WithError(err).Error("failed to claim VHTLC")
		}
	}()
	return swap.Invoice, nil
}

func (s *Service) waitAndClaimVHTLC(
	ctx context.Context, swapId string, preimage []byte, vhtlcOpts *vhtlc.Opts,
) (string, error) {
	wsClient := s.boltzSvc
	if s.boltzSvc.URL == boltzURLByNetwork[arklib.BitcoinRegTest.Name] {
		wsClient = &boltz.Api{WSURL: "http://localhost:9004"}
	}

	ws := wsClient.NewWebsocket()
	{
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		err := ws.Connect()
		for err != nil {
			log.WithError(err).Warn("failed to connect to boltz websocket")
			time.Sleep(time.Second)
			log.Debug("reconnecting...")
			err = ws.Connect()
			if ctx.Err() != nil {
				return "", fmt.Errorf("timeout while connecting to websocket: %v", ctx.Err())
			}
		}

		err = ws.Subscribe([]string{swapId})
		for err != nil {
			log.WithError(err).Warn("failed to subscribe for swap events")
			time.Sleep(time.Second)
			log.Debug("retrying...")
			err = ws.Subscribe([]string{swapId})
		}
	}

	var txid string
	for update := range ws.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		confirmed := false
		switch parsedStatus {
		case boltz.TransactionMempool:
			confirmed = true
		case boltz.InvoiceFailedToPay, boltz.TransactionFailed, boltz.TransactionLockupFailed:
			return "", fmt.Errorf("failed to receive payment: %s", update.Status)
		}
		if confirmed {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			interval := 200 * time.Millisecond
			log.Debug("claiming VHTLC with preimage...")
			if err := utils.Retry(ctx, interval, func(ctx context.Context) (bool, error) {
				var err error
				txid, err = s.claimVHTLC(ctx, preimage, *vhtlcOpts)
				if err != nil {
					if errors.Is(err, ErrorNoVtxosFound) {
						return false, nil
					}
					return false, err
				}

				return true, nil
			}); err != nil {
				return "", err
			}
			log.Debugf("successfully claimed VHTLC with tx: %s", txid)
			break
		}
	}

	return txid, nil
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

func (s *Service) getVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *arklib.AbsoluteLocktime,
	unilateralClaimDelayParam *arklib.RelativeLocktime,
	unilateralRefundDelayParam *arklib.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *arklib.RelativeLocktime,
) (string, *vhtlc.VHTLCScript, *vhtlc.Opts, error) {
	receiverPubkeySet := receiverPubkey != nil
	senderPubkeySet := senderPubkey != nil
	if receiverPubkeySet == senderPubkeySet {
		return "", nil, nil, fmt.Errorf("only one of receiver and sender pubkey must be set")
	}
	if !receiverPubkeySet {
		receiverPubkey = s.publicKey
	}
	if !senderPubkeySet {
		senderPubkey = s.publicKey
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
	vHTLC, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", nil, nil, err
	}

	encodedAddr, err := vHTLC.Address(cfg.Network.Addr, cfg.SignerPubKey)
	if err != nil {
		return "", nil, nil, err
	}

	return encodedAddr, vHTLC, &opts, nil
}

func (s *Service) getVHTLCFunds(ctx context.Context, vhtlcOpts []vhtlc.Opts) ([]types.Vtxo, error) {
	var allVtxos []types.Vtxo
	for _, opt := range vhtlcOpts {
		vHTLC, err := vhtlc.NewVHTLCScript(opt)
		if err != nil {
			return nil, err
		}

		tapKey, _, err := vHTLC.TapTree()
		if err != nil {
			return nil, err
		}

		outScript, err := script.P2TRScript(tapKey)
		if err != nil {
			return nil, err
		}

		vtxosRequest := indexer.GetVtxosRequestOption{}
		if err := vtxosRequest.WithScripts([]string{hex.EncodeToString(outScript)}); err != nil {
			return nil, err
		}
		resp, err := s.indexerClient.GetVtxos(ctx, vtxosRequest)
		if err != nil {
			return nil, err
		}
		allVtxos = append(allVtxos, resp.Vtxos...)
	}

	return allVtxos, nil
}

func (s *Service) claimVHTLC(
	ctx context.Context, preimage []byte, vhtlcOpts vhtlc.Opts,
) (string, error) {
	vtxos, err := s.getVHTLCFunds(ctx, []vhtlc.Opts{vhtlcOpts})
	if err != nil {
		return "", err
	}
	if len(vtxos) == 0 {
		return "", ErrorNoVtxosFound
	}

	vtxo := &vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	// self send output
	_, myAddr, _, _, _, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", err
	}

	decodedAddr, err := arklib.DecodeAddressV0(myAddr)
	if err != nil {
		return "", err
	}

	pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	cfg, err := s.GetConfigData(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get config data: %w", err)
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	claimTapscript, err := vtxoScript.ClaimTapscript()
	if err != nil {
		return "", err
	}

	checkpointScript, err := hex.DecodeString(cfg.CheckpointTapscript)
	if err != nil {
		return "", err
	}

	arkTx, checkpoints, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          claimTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
		checkpointScript,
	)
	if err != nil {
		return "", err
	}

	signTransaction := func(tx *psbt.Packet) (string, error) {
		// add the preimage to the checkpoint input
		if err := txutils.AddConditionWitness(0, tx, wire.TxWitness{preimage}); err != nil {
			return "", err
		}

		encoded, err := tx.B64Encode()
		if err != nil {
			return "", err
		}

		return s.SignTransaction(ctx, encoded)
	}

	signedArkTx, err := signTransaction(arkTx)
	if err != nil {
		return "", err
	}

	checkpointTxs := make([]string, 0, len(checkpoints))
	for _, ptx := range checkpoints {
		tx, err := ptx.B64Encode()
		if err != nil {
			return "", err
		}
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, signedCheckpoints, err := s.grpcClient.SubmitTx(ctx, signedArkTx, checkpointTxs)
	if err != nil {
		return "", err
	}

	if err := verifyFinalArkTx(finalArkTx, cfg.SignerPubKey, getInputTapLeaves(arkTx)); err != nil {
		return "", err
	}

	finalCheckpoints, err := verifyAndSignCheckpoints(signedCheckpoints, checkpoints, cfg.SignerPubKey, signTransaction)
	if err != nil {
		return "", err
	}

	err = s.grpcClient.FinalizeTx(ctx, arkTxid, finalCheckpoints)
	if err != nil {
		return "", fmt.Errorf("failed to finalize redeem transaction: %w", err)
	}

	return arkTxid, nil
}

func (s *Service) refundVHTLC(
	ctx context.Context, swapId string, withReceiver bool, vhtlcOpts vhtlc.Opts,
) (string, error) {
	cfg, err := s.GetConfigData(ctx)
	if err != nil {
		return "", err
	}

	vtxos, err := s.getVHTLCFunds(ctx, []vhtlc.Opts{vhtlcOpts})
	if err != nil {
		return "", err
	}
	vtxo := vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	refundTapscript, err := vtxoScript.RefundTapscript(withReceiver)
	if err != nil {
		return "", err
	}

	dest, err := txscript.PayToTaprootScript(vhtlcOpts.Sender)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	checkpointScript, err := hex.DecodeString(cfg.CheckpointTapscript)
	if err != nil {
		return "", err
	}

	refundTx, checkpointPtxs, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          refundTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: dest,
			},
		},
		checkpointScript,
	)
	if err != nil {
		return "", err
	}

	signTransaction := func(tx *psbt.Packet) (string, error) {
		encoded, err := tx.B64Encode()
		if err != nil {
			return "", err
		}
		return s.SignTransaction(ctx, encoded)
	}

	signedRefundTx, err := signTransaction(refundTx)
	if err != nil {
		return "", err
	}

	if withReceiver {
		signedRefundTx, err = s.boltzRefundSwap(swapId, signedRefundTx)
		if err != nil {
			return "", err
		}
	}

	checkpointTxs := make([]string, 0, len(checkpointPtxs))
	for _, ptx := range checkpointPtxs {
		tx, err := ptx.B64Encode()
		if err != nil {
			return "", err
		}
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, signedCheckpoints, err := s.grpcClient.SubmitTx(ctx, signedRefundTx, checkpointTxs)
	if err != nil {
		return "", err
	}

	if err := verifyFinalArkTx(finalArkTx, cfg.SignerPubKey, getInputTapLeaves(refundTx)); err != nil {
		return "", err
	}

	// verify and sign the checkpoints
	finalCheckpoints, err := verifyAndSignCheckpoints(signedCheckpoints, checkpointPtxs, cfg.SignerPubKey, signTransaction)
	if err != nil {
		return "", err
	}

	err = s.grpcClient.FinalizeTx(ctx, arkTxid, finalCheckpoints)
	if err != nil {
		return "", fmt.Errorf("failed to finalize redeem transaction: %w", err)
	}

	return arkTxid, nil
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

// getInputTapLeaves returns a map of input index to tapscript leaf
// if the input has no tapscript leaf, it is not included in the map
func getInputTapLeaves(tx *psbt.Packet) map[int]txscript.TapLeaf {
	tapLeaves := make(map[int]txscript.TapLeaf)
	for inputIndex, input := range tx.Inputs {
		if input.TaprootLeafScript == nil {
			continue
		}
		tapLeaves[inputIndex] = txscript.NewBaseTapLeaf(input.TaprootLeafScript[0].Script)
	}
	return tapLeaves
}

func verifyAndSignCheckpoints(signedCheckpoints []string, myCheckpoints []*psbt.Packet, arkSigner *btcec.PublicKey, sign func(tx *psbt.Packet) (string, error)) ([]string, error) {
	finalCheckpoints := make([]string, 0, len(signedCheckpoints))
	for _, checkpoint := range signedCheckpoints {
		signedCheckpointPtx, err := psbt.NewFromRawBytes(strings.NewReader(checkpoint), true)
		if err != nil {
			return nil, err
		}

		// search for the checkpoint tx we initially created
		var myCheckpointTx *psbt.Packet
		for _, chk := range myCheckpoints {
			if chk.UnsignedTx.TxID() == signedCheckpointPtx.UnsignedTx.TxID() {
				myCheckpointTx = chk
				break
			}
		}
		if myCheckpointTx == nil {
			return nil, fmt.Errorf("checkpoint tx not found")
		}

		// verify the server has signed the checkpoint tx
		err = verifyInputSignatures(signedCheckpointPtx, arkSigner, getInputTapLeaves(myCheckpointTx))
		if err != nil {
			return nil, err
		}

		finalCheckpoint, err := sign(signedCheckpointPtx)
		if err != nil {
			return nil, fmt.Errorf("failed to sign checkpoint transaction: %w", err)
		}

		finalCheckpoints = append(finalCheckpoints, finalCheckpoint)
	}

	return finalCheckpoints, nil
}

func verifyFinalArkTx(finalArkTx string, arkSigner *btcec.PublicKey, expectedTapLeaves map[int]txscript.TapLeaf) error {
	finalArkPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalArkTx), true)
	if err != nil {
		return err
	}

	// verify that the ark signer has signed the ark tx
	err = verifyInputSignatures(finalArkPtx, arkSigner, expectedTapLeaves)
	if err != nil {
		return err
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
