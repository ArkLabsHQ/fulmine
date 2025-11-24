package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
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
	arklib.Bitcoin.Name:          "https://api.ark.boltz.exchange",
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

	swapTimeout uint32

	isInitialized bool
	syncLock      *sync.RWMutex
	syncEvent     *types.SyncEvent
	syncCh        chan types.SyncEvent

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
	refreshDbInterval int64,
) (*Service, error) {
	opts := make([]arksdk.ClientOption, 0)
	if log.IsLevelEnabled(log.DebugLevel) {
		opts = append(opts, arksdk.WithVerbose())
	}

	// force rescan transactions history and (u/v)txos set every refreshDbInterval
	if refreshDbInterval > 0 {
		opts = append(opts, arksdk.WithRefreshDb(time.Duration(refreshDbInterval)*time.Second))
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
			indexerClient:             indexerClient,
			schedulerSvc:              schedulerSvc,
			publicKey:                 nil,
			isInitialized:             true,
			notifications:             make(chan Notification),
			stopBoardingEventListener: make(chan struct{}),
			esploraUrl:                data.ExplorerURL,
			boltzUrl:                  boltzUrl,
			boltzWSUrl:                boltzWSUrl,
			swapTimeout:               swapTimeout,
			walletUpdates:             make(chan WalletUpdate),
			syncLock:                  &sync.RWMutex{},
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
		syncLock:                  &sync.RWMutex{},
	}

	return svc, nil
}

func (s *Service) IsInitialized() bool {
	return s.isInitialized
}

func (s *Service) IsSynced() (bool, error) {
	if s.syncEvent == nil {
		return false, nil
	}
	return s.syncEvent.Synced, s.syncEvent.Err
}

func (s *Service) GetSyncedUpdate() <-chan types.SyncEvent {
	if s.syncEvent != nil {
		ch := make(chan types.SyncEvent, 1)
		go func() { ch <- *s.syncEvent }()
		return ch
	}

	return s.syncCh
}

func (s *Service) GetWalletUpdates() <-chan WalletUpdate {
	return s.walletUpdates
}

func (s *Service) SetupFromMnemonic(
	ctx context.Context, serverUrl, password, mnemonic string,
) error {
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return s.Setup(ctx, serverUrl, password, privateKey)
}

func (s *Service) Setup(ctx context.Context, serverUrl, password, privateKey string) (err error) {
	if s.isInitialized {
		return errors.New("wallet already initialized")
	}

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
		pollingInterval = 2 * time.Second
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
	s.isInitialized = true

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

	if s.schedulerSvc != nil {
		s.schedulerSvc.Stop()
		log.Info("scheduler stopped")
	}

	if s.externalSubscription != nil {
		s.externalSubscription.stop()
	}

	// close boarding event listener
	s.stopBoardingEventListener <- struct{}{}
	close(s.stopBoardingEventListener)
	s.stopBoardingEventListener = make(chan struct{})

	s.syncEvent = nil
	if s.syncCh != nil {
		close(s.syncCh)
		s.syncCh = nil
	}

	go func() {
		s.walletUpdates <- WalletUpdate{Type: "lock"}
	}()

	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	if !s.isInitialized {
		return fmt.Errorf("service not initialized")
	}
	if !s.ArkClient.IsLocked(ctx) {
		return nil
	}

	s.syncCh = make(chan types.SyncEvent, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		s.syncLock.Lock()
		defer s.syncLock.Unlock()
		ev := <-s.ArkClient.IsSynced(context.Background())
		s.syncEvent = &ev
		s.syncCh <- ev
		wg.Done()
	}()

	if err := s.Unlock(ctx, password); err != nil {
		return err
	}

	s.schedulerSvc.Start()
	log.Info("scheduler started")

	arkConfig, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}
	settings, err := s.dbSvc.Settings().GetSettings(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get settings")
		return err
	}

	// This go routine takes care of scheduling the next settlement and restore the watch
	// for the subscribed addresses.
	// All operations that require the sdk client to be synced must stay here.
	// TODO: Improve by handling the errors instead of just logging them.
	go func() {
		// We must wait for the client to be synced before doing anything.
		wg.Wait()

		// Do nothing here if restore failed.
		if s.syncEvent == nil {
			return
		}

		// Schedule next settlement for the current vtxo set.
		nextExpiry, err := s.computeNextExpiry(context.Background(), arkConfig)
		if err != nil {
			log.WithError(err).Error("failed to compute next expiry")
		}

		if nextExpiry != nil {
			if err := s.scheduleNextSettlement(*nextExpiry, arkConfig); err != nil {
				log.WithError(err).Error("failed to schedule next settlement")
			}
		}

		if s.boltzSvc == nil {
			url := s.boltzUrl
			wsUrl := s.boltzWSUrl
			if url == "" {
				url = boltzURLByNetwork[arkConfig.Network.Name]
			}
			if wsUrl == "" {
				wsUrl = boltzURLByNetwork[arkConfig.Network.Name]
			}
			s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}
		}

		// Resume pending swap refunds.
		go s.resumePendingSwapRefunds(ctx)

		// Restore watch of our and tracked addresses.
		_, offchainAddrses, boardingAddresses, _, err := s.GetAddresses(context.Background())
		if err != nil {
			log.WithError(err).Error("failed to get addresses")
		}

		scripts, err := offchainAddressesPkScripts(offchainAddrses)
		if err != nil {
			log.WithError(err).Error("failed to decode offchain address")
		}

		log.Debugf("len of scripts %d", len(scripts))

		_, err = s.dbSvc.SubscribedScript().Add(context.Background(), scripts)
		if err != nil {
			log.Debugf("cannot listen to scripts %+v", err)
		}

		s.externalSubscription = newSubscriptionHandler(
			settings.ServerUrl, s.dbSvc.SubscribedScript(), s.handleAddressEventChannel(arkConfig),
		)

		if err := s.externalSubscription.start(); err != nil {
			log.WithError(err).Error("failed to start external subscription")
		}

		if arkConfig.UtxoMaxAmount != 0 {
			go s.subscribeForBoardingEvent(ctx, boardingAddresses, arkConfig)
		}

		// Load delegate signer key.
		prvkeyStr, err := s.Dump(context.Background())
		if err != nil {
			log.WithError(err).Error("failed to get delegate signer key")
		}

		buf, err := hex.DecodeString(prvkeyStr)
		if err != nil {
			log.WithError(err).Error("failed to decode delegate signer key")
		}

		_, pubkey := btcec.PrivKeyFromBytes(buf)
		s.publicKey = pubkey
	}()

	// This go routine takes care of establishing the LN connection, if configured.
	// TODO: Improve by handling the error instead of just logging it.
	go func() {
		if settings.LnConnectionOpts != nil {
			log.Debug("connecting to LN node...")
			if err = s.connectLN(ctx, settings.LnConnectionOpts); err != nil {
				log.WithError(err).Error("failed to connect to LN node")
			}
		}
	}()

	url := s.boltzUrl
	wsUrl := s.boltzWSUrl
	if url == "" {
		url = boltzURLByNetwork[arkConfig.Network.Name]
	}
	if wsUrl == "" {
		wsUrl = boltzURLByNetwork[arkConfig.Network.Name]
	}
	s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}

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

	if s.schedulerSvc != nil {
		s.schedulerSvc.Stop()
		log.Info("scheduler stopped")
	}

	if s.externalSubscription != nil {
		s.externalSubscription.stop()
	}

	s.isInitialized = false
	s.syncEvent = nil
	if s.syncCh != nil {
		close(s.syncCh)
		s.syncCh = nil
	}
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
	if !s.isInitialized {
		return nil, fmt.Errorf("service not initialized")
	}
	return s.indexerClient.GetCommitmentTx(ctx, roundId)
}

func (s *Service) GetVirtualTxs(ctx context.Context, txids []string) ([]string, error) {
	if !s.isInitialized {
		return nil, fmt.Errorf("service not initialized")
	}

	resp, err := s.indexerClient.GetVirtualTxs(ctx, txids)
	if err != nil {
		return nil, err
	}

	return resp.Txs, nil
}

func (s *Service) Settle(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	commitmentTxid, err := s.ArkClient.Settle(ctx)
	if err != nil {
		return "", err
	}

	s.schedulerSvc.CancelNextSettlement()

	return commitmentTxid, nil
}

func (s *Service) SendOnChain(ctx context.Context, addr string, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	commitmentTxid, err := s.CollaborativeExit(ctx, addr, amount, false)
	if err != nil {
		return "", err
	}

	s.schedulerSvc.CancelNextSettlement()

	return commitmentTxid, nil
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

	go func() {
		if err := s.dbSvc.VHTLC().Add(context.Background(), domain.NewVhtlc(opts)); err != nil {
			log.WithError(err).Error("failed to add vhtlc")
			return
		}

		log.Debugf("added new vhtlc %s", vhtlcId)
	}()

	return encodedAddr, vhtlcId, vHTLCScript, nil
}

func (s *Service) ListVHTLC(
	ctx context.Context, vhtlc_id string,
) ([]types.Vtxo, []domain.Vhtlc, error) {
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
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

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

func (s *Service) ClaimVHTLC(
	ctx context.Context, preimage []byte, vhtlc_id string,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

	return swapHandler.ClaimVHTLC(ctx, preimage, vhtlc.Opts)
}

func (s *Service) RefundVHTLC(
	ctx context.Context, swapId, vhtlc_id string, withReceiver bool,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

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
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

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
	if err != nil {
		return "", fmt.Errorf("failed to create reverse swap: %v", err)
	}

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
func (s *Service) IncreaseOutboundCapacity(
	ctx context.Context, amount uint64,
) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	boltzApi := s.boltzSvc

	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

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

	scripts, err := offchainAddressesPkScripts(addresses)
	if err != nil {
		return err
	}

	return s.externalSubscription.subscribe(ctx, scripts)
}

func (s *Service) UnsubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	scripts, err := offchainAddressesPkScripts(addresses)
	if err != nil {
		return err
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
		return "", fmt.Errorf("delegate service not initialized")
	}

	return hex.EncodeToString(s.publicKey.SerializeCompressed()), nil
}

func (s *Service) WatchAddressForRollover(
	ctx context.Context, address, destinationAddress string, taprootTree []string,
) error {
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
		return true
	}

	return s.ArkClient.IsLocked(ctx)
}

func (s *Service) GetInvoice(ctx context.Context, amount uint64) (SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return SwapResponse{}, err
	}

	boltzApi := s.boltzSvc
	swapHandler := swap.NewSwapHandler(
		s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout,
	)

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

func (s *Service) PayInvoice(ctx context.Context, invoice string) (*SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	boltzApi := s.boltzSvc

	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapHandler, swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := swapHandler.PayInvoice(ctx, invoice, unilateralRefund)
	if err != nil {
		return nil, err
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
			RedeemTxId:  swapDetails.RedeemTxid,
			Status:      swapStatus,
		})

		if dbErr != nil {
			log.WithError(dbErr).Error("failed to add swap to db")
			return
		}

	}()

	return &SwapResponse{
		TxId:       swapDetails.TxId,
		SwapStatus: swapStatus,
		Invoice:    swapDetails.Invoice,
	}, err
}

func (s *Service) PayOffer(ctx context.Context, offer string) (*SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	configData, err := s.GetConfigData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config data: %v", err)
	}

	boltzApi := s.boltzSvc
	var lightningUrl string

	if configData.Network.Name == arklib.BitcoinRegTest.Name {
		boltzUrl, err := url.Parse(s.boltzSvc.URL)
		if err != nil {
			return nil, err
		}
		host := boltzUrl.Hostname()
		boltzUrl.Host = fmt.Sprintf("%s:%d", host, 9005)
		lightningUrl = boltzUrl.String()
	}

	swapHandler := swap.NewSwapHandler(
		s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout,
	)

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapHandler, swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := swapHandler.PayOffer(ctx, offer, lightningUrl, unilateralRefund)

	if err != nil {
		return nil, err
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
			RedeemTxId:  swapDetails.RedeemTxid,
			Timestamp:   swapDetails.Timestamp,
			Status:      swapStatus,
		})

		if dbErr != nil {
			log.WithError(dbErr).Error("failed to add swap to db")
			return
		}

	}()

	return &SwapResponse{
		TxId:       swapDetails.TxId,
		SwapStatus: swapStatus,
		Invoice:    swapDetails.Invoice,
	}, err
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

func (s *Service) isInitializedAndUnlocked(ctx context.Context) error {
	if !s.isInitialized {
		return fmt.Errorf("service not initialized")
	}

	if s.IsLocked(ctx) {
		return fmt.Errorf("service is locked")
	}

	return nil
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
			boardingDelay := time.Duration(data.BoardingExitDelay.Seconds()) * time.Second
			boardingExpiry := tx.CreatedAt.Add(boardingDelay)
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

func (s *Service) scheduleNextSettlement(at time.Time, data *types.Config) error {
	task := func() {
		_, err := s.Settle(context.Background())
		if err != nil {
			log.WithError(err).Warn("failed to auto claim")
		}
	}

	// TODO: Fetch GetInfo to know if there's any scheduled session close to "at",
	// otherwise keep this as fallback strategy, ie. schedule the settlement 2 session durations
	// before "at"
	sessionDuration := time.Duration(data.SessionDuration) * time.Second
	at = at.Add(-2 * sessionDuration)
	now := time.Now()
	nextSettlement := s.schedulerSvc.WhenNextSettlement()

	// Checking if "at" is after now is a safe guard against buggish time values.
	if !nextSettlement.IsZero() && at.After(now) && at.After(s.schedulerSvc.WhenNextSettlement()) {
		log.Debugf(
			"scheduling next settlement at %s skipped - one already set at %s",
			at.Format(time.RFC3339), s.schedulerSvc.WhenNextSettlement().Format(time.RFC3339),
		)
		return nil
	}

	if err := s.schedulerSvc.ScheduleNextSettlement(at, task); err != nil {
		return err
	}
	// If at is before now the settlement is executed immediately and no logs need to be printed.
	if at.After(now) {
		log.Debugf("scheduled next settlement at %s", at.Format(time.RFC3339))
	}
	return nil
}

// subscribeForBoardingEvent aims to update the scheduled settlement
// by checking for spent and new vtxos on the given boarding address
func (s *Service) subscribeForBoardingEvent(ctx context.Context, addresses []string, cfg *types.Config) {
	eventsCh := s.GetUtxoEventChannel(ctx)
	boardingScripts, err := onchainAddressesPkScripts(addresses, cfg.Network)
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

				if slices.Contains(boardingScripts, utxo.Script) {
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
		at := time.Unix(int64(refundLT), 0)
		if err := s.schedulerSvc.ScheduleRefundAtTime(at, unilateral); err != nil {
			return err
		}
		log.Debugf("scheduled unilateral refund of swap %s at %s", swapId, at.Format(time.RFC3339))
	} else {
		if err := s.schedulerSvc.ScheduleRefundAtHeight(uint32(refundLT), unilateral); err != nil {
			return err
		}
		log.Debugf("scheduling vhtlc refund of swap %s at height %d", swapId, int64(refundLT))
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
	swapHandler := swap.NewSwapHandler(s.ArkClient, s.grpcClient, s.indexerClient, boltzApi, s.publicKey, s.swapTimeout)

	for _, swap := range swaps {

		if swap.Status == domain.SwapFailed && swap.RedeemTxId == "" && swap.From == boltz.CurrencyArk {
			if err := s.scheduleSwapRefund(swapHandler, swap.Id, swap.Vhtlc.Opts); err != nil {
				log.WithError(err).WithField("swap_id", swap.Id).Warn("failed to reschedule refund task")
			}
		}

	}
}
