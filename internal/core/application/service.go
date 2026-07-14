package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	client "github.com/arkade-os/arkd/pkg/client-lib"
	singlekeywallet "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey"
	filestore "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey/store/file"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientstore "github.com/arkade-os/arkd/pkg/client-lib/store"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/contract"
	"github.com/arkade-os/go-sdk/types"
	"github.com/arkade-os/go-sdk/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/input"
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

// networkNameToParams converts arklib network name to chaincfg.Params
func networkNameToParams(networkName string) *chaincfg.Params {
	switch networkName {
	case arklib.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case arklib.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	case arklib.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	case arklib.BitcoinSigNet.Name, arklib.BitcoinMutinyNet.Name:
		return &chaincfg.SigNetParams
	default:
		// Default to regtest for safety
		return &chaincfg.RegressionNetParams
	}
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

	arksdk.Wallet
	configStore  clientTypes.ConfigStore
	dbSvc        ports.RepoManager
	schedulerSvc ports.SchedulerService
	boltzSvc     *boltz.Api
	swapHandler  *swap.SwapHandler

	publicKey  *btcec.PublicKey
	privateKey *btcec.PrivateKey

	// emulatorPubKey is the server-configured non-interactive claim tapscript
	// key. Nil when unset, which disables non-interactive claims.
	emulatorPubKey *btcec.PublicKey

	esploraUrl string
	boltzUrl   string
	boltzWSUrl string

	swapTimeout uint32

	isInitialized bool
	walletReady   atomic.Bool // true once UnlockNode has populated publicKey/privateKey/swapHandler
	syncLock      *sync.RWMutex
	syncEvent     *types.SyncEvent
	syncCh        chan types.SyncEvent

	externalSubscription *subscriptionHandler

	walletUpdates chan WalletUpdate

	// Notification channels
	notifications chan Notification

	// vtxoListenerCancel stops subscribeForVtxoEvent. A context cancel is idempotent
	// and non-blocking, so lock and unlock-rollback can stop the listener without the
	// unbuffered-channel hand-off that could hang if it had already exited.
	vtxoListenerCancel context.CancelFunc

	// renewing is a single-flight guard so that, while we are settling to renew
	// already-expired vtxos, concurrent vtxo events don't pile up extra settles.
	renewing atomic.Bool

	// callback functions to stop and start delegate service
	onUnlock func()
	onLock   func()
}

type Notification struct {
	indexer.TxData
	Addrs       []string
	NewVtxos    []clientTypes.Vtxo
	SpentVtxos  []clientTypes.Vtxo
	Checkpoints map[string]indexer.TxData
}

type SwapResponse struct {
	TxId       string
	SwapStatus domain.SwapStatus
	Invoice    string
}

type DelegateConfig struct {
	Enabled bool
	Fee     uint64
}

func NewServices(
	buildInfo BuildInfo,
	datadir string,
	dbSvc ports.RepoManager,
	schedulerSvc ports.SchedulerService,
	esploraUrl, boltzUrl, boltzWSUrl string, swapTimeout uint32,
	refreshDbInterval int64,
	delegateConfig DelegateConfig,
	emulatorPubkeyHex string,
) (*Service, *DelegateService, error) {
	var emulatorPubKey *btcec.PublicKey
	if emulatorPubkeyHex != "" {
		pubBytes, err := hex.DecodeString(emulatorPubkeyHex)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid emulator pubkey hex: %w", err)
		}
		emulatorPubKey, err = btcec.ParsePubKey(pubBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse emulator pubkey: %w", err)
		}
	}

	svc, err := newService(
		buildInfo, datadir, dbSvc, schedulerSvc, refreshDbInterval,
		esploraUrl, boltzUrl, boltzWSUrl, swapTimeout, emulatorPubKey,
	)
	if err != nil {
		return nil, nil, err
	}

	if delegateConfig.Enabled {
		delegateSvc := newDelegateService(svc, delegateConfig.Fee)
		svc.onUnlock = func() {
			delegateSvc.start()
		}
		svc.onLock = func() {
			delegateSvc.Stop()
		}
		return svc, delegateSvc, nil
	}

	return svc, nil, nil
}

func newService(
	buildInfo BuildInfo,
	datadir string,
	dbSvc ports.RepoManager,
	schedulerSvc ports.SchedulerService,
	refreshDbInterval int64,
	esploraUrl, boltzUrl, boltzWSUrl string, swapTimeout uint32,
	emulatorPubKey *btcec.PublicKey,
) (*Service, error) {
	walletStore, err := filestore.NewStore(datadir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet store: %w", err)
	}
	singleKeyWallet, err := singlekeywallet.NewIdentity(walletStore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet: %w", err)
	}

	// Same file-backed store the SDK opens internally; the SDK no longer
	// exposes its config store, so open a second handle to persist updates.
	clientStore, err := clientstore.NewStore(clientstore.Config{
		ConfigStoreType: clientTypes.FileStore,
		BaseDir:         datadir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config store: %w", err)
	}
	configStore := clientStore.ConfigStore()

	opts := []arksdk.WalletOption{
		arksdk.WithRefreshDbInterval(time.Duration(refreshDbInterval) * time.Second),
		arksdk.WithIdentity(singleKeyWallet),
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		opts = append(opts, arksdk.WithVerbose())
	}
	if arkClient, err := arksdk.LoadWallet(datadir, opts...); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}

		svc := &Service{
			BuildInfo:     buildInfo,
			Wallet:        arkClient,
			configStore:   configStore,
			dbSvc:         dbSvc,
			schedulerSvc:  schedulerSvc,
			publicKey:      nil,
			emulatorPubKey: emulatorPubKey,
			isInitialized:  true,
			notifications: make(chan Notification),
			esploraUrl:    data.ExplorerURL,
			boltzUrl:      boltzUrl,
			boltzWSUrl:    boltzWSUrl,
			swapTimeout:   swapTimeout,
			walletUpdates: make(chan WalletUpdate),
			syncLock:      &sync.RWMutex{},
		}

		if err := svc.RefreshServerConfig(context.Background()); err != nil {
			return nil, err
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

	arkClient, err := arksdk.NewWallet(datadir, opts...)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	svc := &Service{
		BuildInfo:     buildInfo,
		Wallet:        arkClient,
		configStore:    configStore,
		dbSvc:          dbSvc,
		schedulerSvc:   schedulerSvc,
		notifications:  make(chan Notification),
		esploraUrl:     esploraUrl,
		boltzUrl:       boltzUrl,
		boltzWSUrl:     boltzWSUrl,
		swapTimeout:    swapTimeout,
		emulatorPubKey: emulatorPubKey,
		walletUpdates:  make(chan WalletUpdate),
		syncLock:       &sync.RWMutex{},
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

// RefreshServerConfig fetches the current server info and updates the
// persisted config for fields that may change after initial setup
// (forfeit address, forfeit pubkey, checkpoint tapscript).
func (s *Service) RefreshServerConfig(ctx context.Context) error {
	if !s.isInitialized {
		return fmt.Errorf("service not initialized")
	}

	currentCfg, err := s.GetConfigData(ctx)
	if err != nil {
		return fmt.Errorf("failed to read current config: %w", err)
	}

	info, err := s.Client().GetInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get server info: %w", err)
	}

	forfeitPubkeyBuf, err := hex.DecodeString(info.ForfeitPubKey)
	if err != nil {
		return fmt.Errorf("failed to decode forfeit pubkey: %w", err)
	}
	forfeitPubkey, err := btcec.ParsePubKey(forfeitPubkeyBuf)
	if err != nil {
		return fmt.Errorf("failed to parse forfeit pubkey: %w", err)
	}

	// Nothing to do if nothing changed server-side
	if info.ForfeitAddress == currentCfg.ForfeitAddress &&
		forfeitPubkey.IsEqual(currentCfg.ForfeitPubKey) &&
		info.CheckpointTapscript == currentCfg.CheckpointTapscript {
		return nil
	}

	currentCfg.ForfeitAddress = info.ForfeitAddress
	currentCfg.ForfeitPubKey = forfeitPubkey
	currentCfg.CheckpointTapscript = info.CheckpointTapscript

	if err := s.configStore.AddData(ctx, *currentCfg); err != nil {
		return fmt.Errorf("failed to persist updated config: %w", err)
	}

	return nil
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

	var opts []arksdk.InitOption
	if s.esploraUrl != "" {
		opts = append(opts, arksdk.WithExplorerURL(s.esploraUrl))
	}

	if err := s.Init(ctx, validatedServerUrl, privateKey, password, opts...); err != nil {
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
	s.privateKey = prvKey
	s.isInitialized = true

	// Revitilise all Swaps If Present
	if err := s.restoreSwapHistory(ctx); err != nil {
		log.WithError(err).Warnf("failed to restore swap history")
	}

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

	if s.onLock != nil {
		s.onLock()
	}

	if s.schedulerSvc != nil {
		s.schedulerSvc.Stop()
		log.Info("scheduler stopped")
	}

	if s.externalSubscription != nil {
		s.externalSubscription.stop()
	}

	// stop the vtxo event listener (cancel is idempotent and never blocks)
	if s.vtxoListenerCancel != nil {
		s.vtxoListenerCancel()
		s.vtxoListenerCancel = nil
	}

	s.walletReady.Store(false)
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

// unwindFailedUnlock rolls back a partially-completed unlock so the wallet
// returns to a clean locked state and a fresh unlock can retry, instead of being
// stuck "finalizing unlock" until a restart. It runs only from UnlockNode's
// post-sync goroutine after wg.Wait, and LockNode is gated out while walletReady
// is false, so there is no concurrent teardown to race with.
func (s *Service) unwindFailedUnlock() {
	if s.schedulerSvc != nil {
		s.schedulerSvc.Stop()
	}
	if s.externalSubscription != nil {
		s.externalSubscription.stop()
	}
	// stop the vtxo event listener if it launched (cancel is idempotent / non-blocking)
	if s.vtxoListenerCancel != nil {
		s.vtxoListenerCancel()
		s.vtxoListenerCancel = nil
	}

	// Stop the delegate service that onUnlock may have started before the failure;
	// otherwise its event loops keep running against the wallet we re-lock below.
	// Stop is idempotent and non-blocking, so this is safe even on the early paths
	// where onUnlock never ran.
	if s.onLock != nil {
		s.onLock()
	}

	s.walletReady.Store(false)
	s.syncEvent = nil
	if s.syncCh != nil {
		close(s.syncCh)
		s.syncCh = nil
	}

	// Re-lock LAST. s.Lock makes IsLocked() return true, which reopens UnlockNode's
	// guard; tearing down syncCh/syncEvent/walletReady first means a retry that races
	// in right after the lock allocates a fresh syncCh instead of finding this one
	// mid-close (a send on a closed syncCh would panic its sync goroutine).
	if err := s.Lock(context.Background()); err != nil {
		log.WithError(err).Error("failed to re-lock after a failed unlock")
	}
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	if !s.isInitialized {
		return fmt.Errorf("service not initialized")
	}
	if !s.Wallet.IsLocked(ctx) {
		return nil
	}

	// Stays closed until the post-sync goroutine below finishes assembling the
	// wallet, so the unlock window can't expose a nil publicKey/privateKey/swapHandler.
	s.walletReady.Store(false)

	if err := s.Unlock(ctx, password); err != nil {
		return err
	}

	s.schedulerSvc.Start()
	log.Info("scheduler started")

	arkConfig, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	subsHandler, err := newSubscriptionHandler(
		ctx, s.Indexer(), s.dbSvc.SubscribedScript(), s.handleAddressEventChannel(arkConfig),
	)
	if err != nil {
		return err
	}
	s.externalSubscription = subsHandler

	// Arm the sync waiter only now that every synchronous failure path is past.
	// Arming it before Unlock/GetConfigData/newSubscriptionHandler can still fail
	// would leave this goroutine blocked on IsSynced while holding syncLock (a
	// failed unlock never syncs), wedging every later retry. Starting it here can't
	// miss the event: IsSynced replays a completed sync via its syncDone fast-path.
	s.syncCh = make(chan types.SyncEvent, 1)
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		s.syncLock.Lock()
		defer s.syncLock.Unlock()
		ev := <-s.Wallet.IsSynced(context.Background())
		s.syncEvent = &ev
		s.syncCh <- ev
	})

	// This go routine takes care of scheduling the next settlement and restore the watch
	// for the subscribed addresses.
	// All operations that require the sdk client to be synced must stay here.
	// TODO: Improve by handling the errors instead of just logging them.
	go func() {
		// This goroutine outlives UnlockNode, so its request-scoped ctx is likely
		// already canceled by the time wg.Wait returns. Use a detached context for
		// the finalization work below (same reason the vtxo listener detaches): a
		// canceled ctx would make Dump/resumePendingSwapRefunds fail and spuriously
		// trigger unwindFailedUnlock on an unlock the caller already saw succeed.
		finalizeCtx := context.Background()

		// We must wait for the client to be synced before doing anything.
		wg.Wait()

		// Do nothing here if restore failed.
		if s.syncEvent == nil {
			return
		}

		// Load delegate signer key.
		prvkeyStr, err := s.Dump(finalizeCtx)
		if err != nil {
			log.WithError(err).Error("failed to get delegate signer key")
			s.unwindFailedUnlock()
			return
		}

		buf, err := hex.DecodeString(prvkeyStr)
		if err != nil {
			log.WithError(err).Error("failed to decode delegate signer key")
			s.unwindFailedUnlock()
			return
		}

		privkey, pubkey := btcec.PrivKeyFromBytes(buf)
		s.publicKey = pubkey
		s.privateKey = privkey

		if s.onUnlock != nil {
			s.onUnlock()
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
		go s.resumePendingSwapRefunds(finalizeCtx)

		// Detach from the request-scoped ctx: this listener lives for the whole
		// unlocked session (stopped by cancelling its context), and it makes
		// long-lived sdk calls (ListVtxos/Settle) on every refresh. If it kept
		// the UnlockNode ctx, that ctx being canceled after unlock returns would
		// make every refresh fail with "context canceled" and silently stop
		// rescheduling settlements.
		listenerCtx, cancel := context.WithCancel(context.Background())
		s.vtxoListenerCancel = cancel
		go s.subscribeForVtxoEvent(listenerCtx, arkConfig)

		// Schedule next settlement for the current vtxo set. Subsequent updates
		// are handled by subscribeForVtxoEvent and its periodic safety check.
		if err := s.refreshSettlementSchedule(context.Background(), arkConfig); err != nil {
			log.WithError(err).Error("failed to schedule next settlement")
		}

		swapHandler, err := swap.NewSwapHandler(
			s.Wallet, s.boltzSvc, s.esploraUrl, s.privateKey, s.swapTimeout,
		)
		if err != nil {
			log.WithError(err).Error("failed to create swap handler; rolling back unlock")
			s.unwindFailedUnlock()
			return
		}
		s.swapHandler = swapHandler

		// All gate-required fields are populated; open the gate. The atomic store
		// publishes the writes above to any reader that passes the gate.
		s.walletReady.Store(true)

		go s.recoverChainSwaps(context.Background(), arkConfig)

		s.sanitize(context.Background())
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
	s.walletReady.Store(false)
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

func (s *Service) GetAddress(
	ctx context.Context, sats uint64,
) (bip21Addr, offchainAddr, boardingAddr, invoice, pubkey string, err error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", "", "", "", err
	}

	boardingAddr, err = s.NewBoardingAddress(ctx)
	if err != nil {
		return "", "", "", "", "", err
	}
	offchainAddr, err = s.NewOffchainAddress(ctx)
	if err != nil {
		return "", "", "", "", "", err
	}

	bip21Addr = fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)
	pubkey = hex.EncodeToString(s.publicKey.SerializeCompressed())

	if sats == 0 {
		return
	}

	invoiceResponse, err := s.GetInvoice(ctx, sats)
	if err != nil {
		log.WithError(err).Warn("failed to get boltz invoice")
	}

	if invoiceResponse != nil {
		invoice = invoiceResponse.Invoice
		bip21Addr += fmt.Sprintf("&lightning=%s", invoice)
	}
	btc := float64(sats) / 100000000.0
	amount := fmt.Sprintf("%.8f", btc)
	bip21Addr += fmt.Sprintf("&amount=%s", amount)

	return
}

func (s *Service) GetTotalBalance(ctx context.Context) (uint64, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

	balance, err := s.Balance(ctx)
	if err != nil {
		return 0, err
	}

	return balance.OffchainBalance.Total, nil
}

func (s *Service) GetRound(ctx context.Context, roundId string) (*indexer.CommitmentTx, error) {
	if !s.isInitialized {
		return nil, fmt.Errorf("service not initialized")
	}
	return s.Indexer().GetCommitmentTx(ctx, roundId)
}

func (s *Service) GetVirtualTxs(ctx context.Context, txids []string) ([]string, error) {
	if !s.isInitialized {
		return nil, fmt.Errorf("service not initialized")
	}

	resp, err := s.Indexer().GetVirtualTxs(ctx, txids)
	if err != nil {
		return nil, err
	}

	return resp.Txs, nil
}

func (s *Service) GetVHTLCSpendingTx(
	ctx context.Context, vhtlcId string,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlcRecord, err := s.dbSvc.VHTLC().Get(ctx, vhtlcId)
	if err != nil {
		return "", fmt.Errorf("failed to get VHTLC %s: %w", vhtlcId, err)
	}

	tx, _, err := s.swapHandler.GetVHTLCSpendingTx(ctx, vhtlcRecord.Opts, nil)
	return tx, err
}

func (s *Service) GetDelegateTasks(
	ctx context.Context, status domain.DelegateTaskStatus, limit, offset int,
) ([]domain.DelegateTask, error) {
	return s.dbSvc.Delegate().GetAll(ctx, status, limit, offset)
}

func (s *Service) GetDelegateTaskByID(
	ctx context.Context, id string,
) (*domain.DelegateTask, error) {
	return s.dbSvc.Delegate().GetByID(ctx, id)
}

func (s *Service) GetVtxos(ctx context.Context, filterType string) ([]clientTypes.Vtxo, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	opts := []indexer.GetVtxosOption{}

	switch filterType {
	case "spendable":
		opts = append(opts, indexer.WithSpendableOnly())
	case "spent":
		opts = append(opts, indexer.WithSpentOnly())
	case "recoverable":
		opts = append(opts, indexer.WithRecoverableOnly())
	case "all":
	default:
		return nil, fmt.Errorf("invalid filter type: %s", filterType)
	}

	_, offchainAddrs, _, _, err := s.GetAddresses(ctx)
	if err != nil {
		return nil, err
	}

	scripts := make([]string, 0, len(offchainAddrs))
	for _, addr := range offchainAddrs {
		decoded, err := arklib.DecodeAddressV0(addr)
		if err != nil {
			return nil, err
		}
		script, err := script.P2TRScript(decoded.VtxoTapKey)
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, hex.EncodeToString(script))
	}

	opts = append(opts, indexer.WithScripts(scripts))

	resp, err := s.Indexer().GetVtxos(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return resp.Vtxos, nil
}

func (s *Service) Settle(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	commitmentTxid, err := s.Wallet.Settle(ctx)
	if err != nil {
		return "", err
	}

	return commitmentTxid, nil
}

func (s *Service) SendOnChain(ctx context.Context, addr string, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	commitmentTxid, err := s.CollaborativeExit(ctx, addr, amount)
	if err != nil {
		return "", err
	}

	return commitmentTxid, nil
}

func (s *Service) WhenNextSettlement(ctx context.Context) time.Time {
	return s.schedulerSvc.WhenNextSettlement()
}

func (s *Service) GetSwapVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *arklib.AbsoluteLocktime,
	unilateralClaimDelayParam *arklib.RelativeLocktime,
	unilateralRefundDelayParam *arklib.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *arklib.RelativeLocktime,
	nonInteractiveClaimAddress *arklib.Address, // nil means nic disabled
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
		Sender:                               senderKey,
		Receiver:                             receiverKey,
		Server:                               cfg.SignerPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}
	if nonInteractiveClaimAddress == nil && s.emulatorPubKey == nil {
		return "", "", nil, fmt.Errorf("non-interactive claims are disabled: missing EMULATOR_PUBKEY")
	}

	if nonInteractiveClaimAddress.HRP != cfg.Network.Addr {
		return "", "", nil, fmt.Errorf("non-interactive claim address has wrong network")
	}

	pkgScript, err := nonInteractiveClaimAddress.GetPkScript()
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid non-interactive claim address")
	}


	opts.NonInteractiveClaim = &vhtlc.NonInteractiveClaimOpts{
		ReceiverPkScript: pkgScript,
		EmulatorPubKey: s.emulatorPubKey,
	}
	vHTLCScript, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	if err != nil {
		return "", "", nil, err
	}

	encodedAddr, err := vHTLCScript.Address(cfg.Network.Addr)
	if err != nil {
		return "", "", nil, err
	}

	if err := s.registerVHTLCContract(ctx, opts); err != nil {
		return "", "", nil, fmt.Errorf("failed to register vhtlc contract: %w", err)
	}

	// Persist synchronously: the duplicate check above (VHTLC().Get) and callers
	// like ListVHTLC read the record back immediately, so adding it in a detached
	// goroutine raced the read — intermittently letting duplicates through and
	// making the record briefly invisible (flaky e2e: TestVHTLC dedup and
	// TestSettleVHTLCByDelegateRefundWithOutpoint).
	if err := s.dbSvc.VHTLC().Add(ctx, domain.NewVhtlc(opts)); err != nil {
		return "", "", nil, fmt.Errorf("failed to add vhtlc: %w", err)
	}
	log.Debugf("added new vhtlc %s", vhtlcId)

	return encodedAddr, vhtlcId, vHTLCScript, nil
}

// registerVHTLCContract mirrors a freshly-created VHTLC into the go-sdk
// contract store so that wallet.SignTransaction can resolve the script and
// route signing through the wallet identity.
func (s *Service) registerVHTLCContract(ctx context.Context, opts vhtlc.Opts) error {
	args := contract.VHTLCContractArgs{
		PreimageHash:                         opts.PreimageHash,
		RefundLocktime:                       opts.RefundLocktime,
		UnilateralClaimDelay:                 opts.UnilateralClaimDelay,
		UnilateralRefundDelay:                opts.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: opts.UnilateralRefundWithoutReceiverDelay,
	}

	// contract manager expects only the external counterparty key: 
	// the owned side is derived from the wallet identity.
	ownsSender := s.publicKey.IsEqual(opts.Sender)
	ownsReceiver := s.publicKey.IsEqual(opts.Receiver)
	switch {
	case ownsSender && ownsReceiver:
		log.Debugf("skipping contract registration: wallet owns both vhtlc keys")
		return nil
	case ownsSender:
		args.Receiver = opts.Receiver
	case ownsReceiver:
		args.Sender = opts.Sender
	default:
		log.Debugf("skipping contract registration: wallet owns neither vhtlc key")
		return nil
	}

	contractType := types.ContractTypeVHTLC
	if opts.NonInteractiveClaim != nil {
		contractType = types.ContractTypeNonInteractiveVHTLC
		args.NonInteractiveReceiver = opts.NonInteractiveClaim.ReceiverPkScript
		args.NonInteractiveEmulator = opts.NonInteractiveClaim.EmulatorPubKey
	}
	if _, err := s.Wallet.ContractManager().NewContract(
		ctx, contractType, contract.WithParams(args),
	); err != nil {
		return fmt.Errorf("persist vhtlc contract: %w", err)
	}
	return nil
}

// SendOffChain sends to the given receivers off-chain. If a receiver address
// matches a persisted VHTLC with the non-interactive claim option, the VHTLC
// tap tree is attached to that output of the funding tx so a claimer daemon
// can locate the covenant claim leaf.
func (s *Service) SendOffChain(
	ctx context.Context, receivers []clientTypes.Receiver, sendOpts ...arksdk.SendOffChainOption,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	pkScripts := make([]string, 0, len(receivers))
	for _, r := range receivers {
		decoded, err := arklib.DecodeAddressV0(r.To)
		if err != nil {
			continue
		}
		pkScript, err := script.P2TRScript(decoded.VtxoTapKey)
		if err != nil {
			continue
		}
		pkScripts = append(pkScripts, hex.EncodeToString(pkScript))
	}

	tapTrees := make(map[string][]byte)
	if len(pkScripts) > 0 {
		mgr := s.ContractManager()
		contracts, err := mgr.GetContracts(ctx, contract.WithScripts(pkScripts))
		if err != nil {
			return "", err
		}
		
		for _, c := range contracts {
			if c.Type != types.ContractTypeNonInteractiveVHTLC {
				continue
			}
			h, err := mgr.GetHandler(ctx, c)
			if err != nil {
				return "", fmt.Errorf("get handler for contract %s: %w", c.Script, err)
			}
			tapscripts, err := h.GetTapscripts(c)
			if err != nil {
				return "", fmt.Errorf("get tapscripts for contract %s: %w", c.Script, err)
			}
			encoded, err := txutils.TapTree(tapscripts).Encode()
			if err != nil {
				return "", fmt.Errorf("encode taptree for contract %s: %w", c.Script, err)
			}
			tapTrees[c.Script] = encoded
		}
	}
	if len(tapTrees) > 0 {
		sendOpts = append(sendOpts, client.WithTxOutsTaprootTree(tapTrees))
	}
	return s.Wallet.SendOffChain(ctx, receivers, sendOpts...)
}

func (s *Service) ListVHTLCs(
	ctx context.Context, vhtlcIds []string,
) ([]clientTypes.Vtxo, []domain.Vhtlc, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, nil, err
	}

	// Return empty list if an empty one is provided
	if len(vhtlcIds) <= 0 {
		return nil, nil, nil
	}

	vhtlcList, err := s.dbSvc.VHTLC().GetByIds(ctx, vhtlcIds)
	if err != nil {
		return nil, nil, err
	}

	vhtlcOpts := make([]vhtlc.Opts, 0, len(vhtlcList))
	for _, v := range vhtlcList {
		vhtlcOpts = append(vhtlcOpts, v.Opts)
	}

	vtxos, err := s.swapHandler.GetVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return nil, nil, err
	}

	return vtxos, vhtlcList, nil
}

func (s *Service) ListVHTLC(
	ctx context.Context, vhtlc_id string,
) ([]clientTypes.Vtxo, []domain.Vhtlc, error) {
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

	vhtlcOpts := make([]vhtlc.Opts, 0, len(vhtlcList))
	for _, v := range vhtlcList {
		vhtlcOpts = append(vhtlcOpts, v.Opts)
	}

	vtxos, err := s.swapHandler.GetVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return nil, nil, err
	}

	return vtxos, vhtlcList, nil
}

func (s *Service) ClaimVHTLC(
	ctx context.Context, preimage []byte, vhtlc_id string, outpoint *clientTypes.Outpoint,
) (string, error) {
	return s.withVhtlc(ctx, vhtlc_id, func(opts vhtlc.Opts) (string, error) {
		return s.swapHandler.ClaimVHTLC(ctx, preimage, opts, outpoint)
	})
}

func (s *Service) RefundVHTLC(
	ctx context.Context, swapId, vhtlc_id string, withReceiver bool, outpoint *clientTypes.Outpoint,
) (string, error) {
	return s.withVhtlc(ctx, vhtlc_id, func(opts vhtlc.Opts) (string, error) {
		return s.swapHandler.RefundSwap(
			ctx, swap.SwapTypeSubmarine, swapId, withReceiver, opts, outpoint,
		)
	})
}

// SettleVHTLCWithClaimPath settles a VHTLC via claim path (revealing preimage) in a batch session.
func (s *Service) SettleVHTLCWithClaimPath(
	ctx context.Context, vhtlcId string, preimage []byte, outpoint *clientTypes.Outpoint,
) (string, error) {
	return s.withVhtlc(ctx, vhtlcId, func(opts vhtlc.Opts) (string, error) {
		return s.swapHandler.SettleVHTLCWithClaimPath(ctx, opts, preimage, outpoint)
	})
}

// SettleVHTLCWithRefundPath settles a VHTLC via refund path in a batch session.
func (s *Service) SettleVHTLCWithRefundPath(
	ctx context.Context, vhtlcId string, outpoint *clientTypes.Outpoint,
) (string, error) {
	return s.withVhtlc(ctx, vhtlcId, func(opts vhtlc.Opts) (string, error) {
		return s.swapHandler.SettleVhtlcWithRefundPath(ctx, opts, outpoint)
	})
}

// SettleVHTLCWithCollaborativeRefundPath settles a VHTLC via delegate refund path.
// The counterparty creates the intent and partial forfeit, and Fulmine acts as delegate to
// complete the batch session.
func (s *Service) SettleVHTLCWithCollaborativeRefundPath(
	ctx context.Context, vhtlcId, intentProof, intentMessage, partialForfeitTx string,
	outpoint *clientTypes.Outpoint,
) (string, error) {
	return s.withVhtlc(ctx, vhtlcId, func(opts vhtlc.Opts) (string, error) {

		delegateSignerSession := tree.NewTreeSignerSession(s.privateKey)
		return s.swapHandler.SettleVHTLCWithCollaborativeRefundPath(
			ctx, opts, partialForfeitTx, intentProof, intentMessage, delegateSignerSession, outpoint,
		)
	})
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

func (s *Service) IsLocked(ctx context.Context) bool {
	if s.Wallet == nil {
		return true
	}

	return s.Wallet.IsLocked(ctx)
}

func (s *Service) GetInvoice(ctx context.Context, amount uint64) (*SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	postProcess := func(swapData swap.Swap) error {
		if swapData.Status != swap.SwapSuccess {
			return nil
		}

		vHTLC := domain.NewVhtlc(*swapData.Opts)

		count, err := s.dbSvc.Swap().Add(context.Background(), []domain.Swap{{
			Id:         swapData.Id,
			Type:       domain.SwapPayment,
			Amount:     swapData.Amount,
			From:       boltz.CurrencyBtc,
			To:         boltz.CurrencyArk,
			Vhtlc:      vHTLC,
			Timestamp:  swapData.Timestamp,
			RedeemTxId: swapData.RedeemTxid,
			Status:     domain.SwapStatus(swapData.Status),
		}})
		if count > 0 {
			log.Debugf("added swap %s", swapData.Id)
		}

		return err
	}

	swapDetails, err := s.swapHandler.GetInvoice(ctx, amount, postProcess)
	if err != nil {
		if strings.Contains(err.Error(), "out of limits") {
			return nil, nil
		}

		return nil, err
	}

	return &SwapResponse{Invoice: swapDetails.Invoice, SwapStatus: domain.SwapPending}, err

}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (*SwapResponse, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := s.swapHandler.PayInvoice(ctx, invoice, unilateralRefund)
	if err != nil {
		return nil, err
	}

	swapStatus := domain.SwapStatus(swapDetails.Status)
	vHTLC := domain.NewVhtlc(*swapDetails.Opts)

	go func() {
		count, err := s.dbSvc.Swap().Add(context.Background(), []domain.Swap{{
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
		}})
		if err != nil {
			log.WithError(err).Error("failed to add swap to db")
			return
		}
		if count > 0 {
			log.Debugf("added swap %s", swapDetails.Id)
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

	unilateralRefund := func(swapData swap.Swap) error {
		err := s.scheduleSwapRefund(swapData.Id, *swapData.Opts)
		return err
	}

	swapDetails, err := s.swapHandler.PayOffer(ctx, offer, lightningUrl, unilateralRefund)

	if err != nil {
		return nil, err
	}

	swapStatus := domain.SwapStatus(swapDetails.Status)
	vHTLC := domain.NewVhtlc(*swapDetails.Opts)

	go func() {
		count, err := s.dbSvc.Swap().Add(context.Background(), []domain.Swap{{
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
		}})
		if err != nil {
			log.WithError(err).Error("failed to add swap to db")
			return
		}
		if count > 0 {
			log.Debugf("added swap %s", swapDetails.Id)
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

// CreateChainSwapArkToBtc initiates Ark → BTC chain swap
func (s *Service) CreateChainSwapArkToBtc(
	_ context.Context,
	amount uint64,
	btcAddress string,
) (*domain.ChainSwap, error) {
	ctx := context.Background()
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	network := networkNameToParams(config.Network.Name)

	eventCallback := func(event swap.ChainSwapEvent) {
		s.handleChainSwapEvent(context.Background(), event)
	}

	unilateralRefund := func(swapId string, opts vhtlc.Opts) error {
		err := s.scheduleChainSwapRefund(swapId, opts)
		return err
	}

	chainSwap, err := s.swapHandler.ChainSwapArkToBtc(
		ctx, amount, btcAddress, network, eventCallback, unilateralRefund,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap: %w", err)
	}

	domainSwap := &domain.ChainSwap{
		Id:                      chainSwap.Id,
		From:                    boltz.CurrencyArk,
		To:                      boltz.CurrencyBtc,
		Amount:                  chainSwap.Amount,
		Status:                  domain.ChainSwapPending,
		ClaimPreimage:           hex.EncodeToString(chainSwap.Preimage),
		UserBtcLockupAddress:    btcAddress,
		BoltzCreateResponseJSON: chainSwap.SwapRespJson,
	}

	log.Infof("Created chain swap %s: Ark → BTC", domainSwap.Id)
	return domainSwap, nil
}

// CreateBtcToArkChainSwap initiates BTC → Ark chain swap
func (s *Service) CreateBtcToArkChainSwap(
	ctx context.Context,
	amount uint64,
) (*domain.ChainSwap, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	network := networkNameToParams(config.Network.Name)

	eventCallback := func(event swap.ChainSwapEvent) {
		s.handleChainSwapEvent(context.Background(), event)
	}

	chainSwap, err := s.swapHandler.ChainSwapBtcToArk(ctx, amount, network, eventCallback)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap: %w", err)
	}

	domainSwap := &domain.ChainSwap{
		Id:                      chainSwap.Id,
		From:                    boltz.CurrencyBtc,
		To:                      boltz.CurrencyArk,
		Amount:                  chainSwap.Amount,
		Status:                  domain.ChainSwapPending,
		ClaimPreimage:           hex.EncodeToString(chainSwap.Preimage),
		UserBtcLockupAddress:    chainSwap.UserBtcLockupAddress,
		BoltzCreateResponseJSON: chainSwap.SwapRespJson,
	}

	log.Infof(
		"Created chain swap %s: BTC → Ark, lockup address: %s",
		domainSwap.Id, chainSwap.UserBtcLockupAddress,
	)
	return domainSwap, nil
}

// handleChainSwapEvent processes typed domain events from ChainSwap
// Implements the DDD pattern: fetch domain entity → call domain method → persist
func (s *Service) handleChainSwapEvent(ctx context.Context, event swap.ChainSwapEvent) {
	switch e := event.(type) {
	case swap.CreateEvent:
		from := boltz.CurrencyArk
		to := boltz.CurrencyBtc
		if !e.IsArkToBtc {
			from = boltz.CurrencyBtc
			to = boltz.CurrencyArk
		}
		domainSwap := &domain.ChainSwap{
			Id:                      e.Id,
			From:                    from,
			To:                      to,
			Amount:                  e.Amount,
			Status:                  domain.ChainSwapPending,
			ClaimPreimage:           hex.EncodeToString(e.Preimage),
			UserBtcLockupAddress:    e.UserBtcLockupAddress,
			BoltzCreateResponseJSON: e.SwapRespJson,
		}
		if err := s.dbSvc.ChainSwaps().Add(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf("failed to persist chain swap: %v", err)
			return
		}
	case swap.UserLockEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for UserLockEvent", e.SwapID)
			return
		}
		domainSwap.UserLocked(e.TxID)

		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf(
				"Failed to update chain swap %s after UserLockEvent", e.SwapID,
			)
		}

	case swap.ServerLockEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for ServerLockEvent", e.SwapID)
			return
		}
		domainSwap.ServerLocked(e.TxID)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf(
				"Failed to update chain swap %s after ServerLockEvent", e.SwapID,
			)
		}

	case swap.ClaimEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for ClaimEvent", e.SwapID)
			return
		}
		domainSwap.Claimed(e.TxID)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after ClaimEvent", e.SwapID)
		}

	case swap.RefundEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for RefundEvent", e.SwapID)
			return
		}
		domainSwap.Refunded(e.TxID)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after RefundEvent", e.SwapID)
		}

	case swap.RefundEventUnilaterally:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for RefundEvent", e.SwapID)
			return
		}
		domainSwap.RefundedUnilaterally(e.TxID)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after RefundEvent", e.SwapID)
		}

	case swap.FailEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get chain swap %s for FailEvent", e.SwapID)
			return
		}
		domainSwap.Failed(e.Error)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after FailEvent", e.SwapID)
		}

	case swap.RefundFailedEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf(
				"Failed to get chain swap %s for RefundFailedEvent", e.SwapID,
			)
			return
		}
		domainSwap.RefundFailed(e.Error)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf(
				"Failed to update chain swap %s after RefundFailedEvent", e.SwapID,
			)
		}

	case swap.UserLockFailedEvent:
		domainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, e.SwapID)
		if err != nil {
			log.WithError(err).Errorf(
				"Failed to get chain swap %s for UserLockFailedEvent", e.SwapID,
			)
			return
		}
		domainSwap.UserLockFailed(e.Error)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *domainSwap); err != nil {
			log.WithError(err).Errorf(
				"Failed to update chain swap %s after UserLockFailedEvent", e.SwapID,
			)
		}

	default:
		log.Warnf("Unknown chain swap event type: %T", e)
	}
}

func (s *Service) ListChainSwaps(
	ctx context.Context, swapIDs []string,
) ([]domain.ChainSwap, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	if len(swapIDs) == 0 {
		return s.dbSvc.ChainSwaps().GetAll(ctx)
	}

	return s.dbSvc.ChainSwaps().GetByIDs(ctx, swapIDs)
}

func (s *Service) RefundChainSwap(ctx context.Context, id string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	chainSwap, err := s.dbSvc.ChainSwaps().Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get chain swap: %w", err)
	}

	if chainSwap.From == boltz.CurrencyBtc && chainSwap.To == boltz.CurrencyArk {
		log.Infof("BTC→ARK refund requested for swap %s", id)

		refundTxid, err := s.swapHandler.RefundBtcToArkSwap(
			ctx, chainSwap.Id, chainSwap.Amount,
			chainSwap.UserLockupTxId, chainSwap.BoltzCreateResponseJSON,
		)
		if err != nil {
			chainSwap.RefundFailed(err.Error())
			if updateErr := s.dbSvc.ChainSwaps().Update(ctx, *chainSwap); updateErr != nil {
				log.WithError(updateErr).Errorf(
					"Failed to update chain swap %s after refund failure", id,
				)
			}
			return fmt.Errorf("BTC→ARK refund failed: %w", err)
		}

		chainSwap.RefundedUnilaterally(refundTxid)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *chainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after refund", id)
			return fmt.Errorf("failed to update chain swap: %w", err)
		}

		log.Infof("BTC→ARK refund successful for swap %s: txid=%s", id, refundTxid)
		return nil

	} else if chainSwap.From == boltz.CurrencyArk && chainSwap.To == boltz.CurrencyBtc {
		// ARK → BTC: Cooperative refund via Boltz API (existing flow)
		log.Infof("Initiating ARK→BTC cooperative refund for swap %s", id)

		swapResp := new(boltz.CreateChainSwapResponse)
		if err := json.Unmarshal([]byte(chainSwap.BoltzCreateResponseJSON), swapResp); err != nil {
			return fmt.Errorf("failed to unmarshal CreateChainSwapResponse: %w", err)
		}

		// nolint
		cfg, _ := s.GetConfigData(ctx)
		preimageBytes, err := hex.DecodeString(chainSwap.ClaimPreimage)
		if err != nil {
			return fmt.Errorf("failed to decode claim preimage: %w", err)
		}
		preimageHash256 := sha256.Sum256(preimageBytes)
		preimageHashHASH160 := input.Ripemd160H(preimageHash256[:])

		boltzReceiverKey, err := parsePubkey(swapResp.LockupDetails.ServerPublicKey)
		if err != nil {
			return fmt.Errorf("invalid Boltz claim public key: %w", err)
		}

		refundLocktime := arklib.AbsoluteLocktime(swapResp.LockupDetails.Timeouts.Refund)
		unilateralClaimDelay := parseLocktime(uint32(
			swapResp.LockupDetails.Timeouts.UnilateralClaim,
		))
		unilateralRefundDelay := parseLocktime(uint32(
			swapResp.LockupDetails.Timeouts.UnilateralRefund,
		))
		unilateralRefundNoReceiverDelay := parseLocktime(uint32(
			swapResp.LockupDetails.Timeouts.UnilateralRefundWithoutReceiver,
		))

		opts := vhtlc.Opts{
			Sender:                               s.publicKey,
			Receiver:                             boltzReceiverKey,
			Server:                               cfg.SignerPubKey,
			PreimageHash:                         preimageHashHASH160,
			RefundLocktime:                       refundLocktime,
			UnilateralClaimDelay:                 unilateralClaimDelay,
			UnilateralRefundDelay:                unilateralRefundDelay,
			UnilateralRefundWithoutReceiverDelay: unilateralRefundNoReceiverDelay,
		}

		unilateralRefund := func(swapId string, opts vhtlc.Opts) error {
			err := s.scheduleChainSwapRefund(swapId, opts)
			return err
		}

		refundTxid, err := s.swapHandler.RefundArkToBTCSwap(
			ctx, chainSwap.Id, opts, unilateralRefund,
		)
		if err != nil {
			chainSwap.RefundFailed(err.Error())
			if updateErr := s.dbSvc.ChainSwaps().Update(ctx, *chainSwap); updateErr != nil {
				log.WithError(updateErr).Errorf(
					"Failed to update chain swap %s after refund failure", id,
				)
			}
			return fmt.Errorf("ARK→BTC cooperative refund failed: %w", err)
		}

		chainSwap.Refunded(refundTxid)
		if err := s.dbSvc.ChainSwaps().Update(ctx, *chainSwap); err != nil {
			log.WithError(err).Errorf("Failed to update chain swap %s after refund", id)
			return fmt.Errorf("failed to update chain swap: %w", err)
		}

		log.Infof("ARK→BTC cooperative refund initiated for swap %s", id)
		return nil

	} else {
		return fmt.Errorf("unsupported swap direction: %s → %s", chainSwap.From, chainSwap.To)
	}
}

func (s *Service) isInitializedAndUnlocked(ctx context.Context) error {
	if !s.isInitialized {
		return fmt.Errorf("service not initialized")
	}

	if s.IsLocked(ctx) {
		return fmt.Errorf("service is locked")
	}

	if s.syncEvent == nil {
		return fmt.Errorf("service is syncing")
	}

	// syncEvent only signals that the sdk finished syncing; UnlockNode's post-sync
	// goroutine then populates publicKey/privateKey/swapHandler. Gate on walletReady
	// too so a request in that window fails cleanly instead of nil-derefing a field.
	if !s.walletReady.Load() {
		return fmt.Errorf("wallet is finalizing unlock")
	}

	return nil
}

// withVhtlc is a helper that performs unlock check and VHTLC fetch, then executes the provided action.
// This eliminates boilerplate across multiple service methods that operate on VHTLCs.
//
// It performs:
//  1. Unlock check via isInitializedAndUnlocked
//  2. VHTLC fetch from database
//  3. Executes the action with the fetched VHTLC opts
//
// Returns the result from the action function.
func (s *Service) withVhtlc(
	ctx context.Context, vhtlcId string, action func(vhtlc.Opts) (string, error),
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlcId)
	if err != nil {
		return "", fmt.Errorf("failed to get VHTLC %s: %w", vhtlcId, err)
	}

	return action(vhtlc.Opts)
}

// restoreSwapHistory gets the swap history from Boltz svc, then:
//   - for every refunded swap, gets the refund txid from the indexer
//   - for every completed reverse swap, gets the claim txid from the indexer
//
// And persists the swaps in the db
func (s *Service) restoreSwapHistory(ctx context.Context) error {
	configData, err := s.GetConfigData(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config data: %v", err)
	}

	boltzApi := s.boltzSvc

	if configData.Network.Name == arklib.BitcoinRegTest.Name {
		boltzUrl, err := url.Parse(s.boltzSvc.URL)
		if err != nil {
			return err
		}
		host := boltzUrl.Hostname()
		boltzUrl.Host = fmt.Sprintf("%s:%d", host, 9005)
		boltzApi.URL = boltzUrl.String()

	}

	myPubkey := hex.EncodeToString(s.publicKey.SerializeCompressed())
	history, err := boltzApi.GetSwapHistory(myPubkey)
	if err != nil {
		return err
	}

	if len(history) <= 0 {
		return nil
	}

	submarineMap := make(map[string]domain.Swap, 0)
	reverseMap := make(map[string]domain.Swap, 0)
	refundedSubmarineSwaps := make([]string, 0)
	successfulReverseSwaps := make([]string, 0)
	for _, record := range history {
		swapDetails := record.RefundDetails
		if record.ClaimDetails != nil {
			swapDetails = record.ClaimDetails
		}

		tree := swapDetails.Tree

		vhtlcScript, err := vhtlc.NewVhtlcScript(
			record.PreimageHash, tree.ClaimLeaf.Output, tree.RefundLeaf.Output,
			tree.RefundLeafWithoutReceiver.Output, tree.UnilateralClaimLeaf.Output,
			tree.UnilateralRefundLeaf.Output, tree.UnilateralRefundWithoutReceiver.Output,
		)
		if err != nil {
			return err
		}

		addr, err := vhtlcScript.Address(configData.Network.Addr)
		if err != nil {
			return err
		}
		if addr != swapDetails.LockupAddress {
			return fmt.Errorf(
				"address mismatch for swap %s: got %s, expected: %s",
				record.Id, addr, swapDetails.LockupAddress,
			)
		}

		// Safe to ignore the error as vhtlcScript.Address calls the same API under the hood
		// nolint
		tapKey, _, _ := vhtlcScript.TapTree()
		buf, err := script.P2TRScript(tapKey)
		if err != nil {
			return err
		}
		outScript := hex.EncodeToString(buf)

		var fundingTxid, redeemTxid string
		var isSubmarineSwap, isReverseSwap bool
		switch {
		case record.From == boltz.CurrencyArk && record.To == boltz.CurrencyBtc:
			isSubmarineSwap = true
			fundingTxid = swapDetails.Transaction.ID
			if boltz.ParseEvent(record.Status) == boltz.TransactionRefunded {
				refundedSubmarineSwaps = append(refundedSubmarineSwaps, outScript)
			}
		case record.From == boltz.CurrencyBtc && record.To == boltz.CurrencyArk:
			isReverseSwap = true
			redeemTxid = swapDetails.Transaction.ID
			if boltz.ParseEvent(record.Status) == boltz.InvoiceSettled {
				successfulReverseSwaps = append(successfulReverseSwaps, outScript)
			}
		}

		swap := domain.Swap{
			Id:          record.Id,
			Status:      convertSwapStatus(record.Status),
			Timestamp:   int64(record.CreatedAt),
			Amount:      swapDetails.Amount,
			To:          record.To,
			From:        record.From,
			Type:        domain.SwapPayment,
			Vhtlc:       domain.NewVhtlc(vhtlcScript.Opts()),
			FundingTxId: fundingTxid,
			RedeemTxId:  redeemTxid,
		}

		if isSubmarineSwap {
			submarineMap[outScript] = swap
		}
		if isReverseSwap {
			reverseMap[outScript] = swap
		}
	}

	if len(refundedSubmarineSwaps) > 0 {
		resp, err := s.Indexer().GetVtxos(ctx, indexer.WithScripts(refundedSubmarineSwaps))
		if err != nil {
			return fmt.Errorf("failed to fetch vtxos for refunded swaps: %s", err)
		}

		for _, vtxo := range resp.Vtxos {
			if !vtxo.Spent {
				continue
			}
			scriptHex := vtxo.Script
			swp, exists := submarineMap[scriptHex]
			if !exists {
				continue
			}

			swp.RedeemTxId = vtxo.ArkTxid
			submarineMap[scriptHex] = swp
		}
	}

	if len(successfulReverseSwaps) != 0 {
		resp, err := s.Indexer().GetVtxos(ctx, indexer.WithScripts(successfulReverseSwaps))
		if err != nil {
			return fmt.Errorf("failed to fetch vtxos for successful reverse swaps: %s", err)
		}

		for _, vtxo := range resp.Vtxos {
			if !vtxo.Spent {
				continue
			}
			scriptHex := vtxo.Script
			swp, exists := reverseMap[scriptHex]
			if !exists {
				continue
			}

			swp.RedeemTxId = vtxo.ArkTxid
			reverseMap[scriptHex] = swp
		}
	}

	// Persist all swaps
	allswaps := make([]domain.Swap, 0)
	for _, swp := range submarineMap {
		allswaps = append(allswaps, swp)
	}
	for _, swp := range reverseMap {
		allswaps = append(allswaps, swp)
	}

	count, err := s.dbSvc.Swap().Add(ctx, allswaps)
	if err != nil {
		return fmt.Errorf("failed to add swaps to db: %s", err)
	}
	if count > 0 {
		log.Infof("restored %d swaps", count)
	}

	return nil
}

func (s *Service) computeNextExpiry(
	ctx context.Context, data *clientTypes.Config,
) (*time.Time, error) {
	var spendableVtxos []clientTypes.Vtxo
	for cursor := ""; ; {
		page, next, err := s.ListVtxos(
			ctx, arksdk.WithSpendableOnly(), arksdk.WithCursor(cursor),
		)
		if err != nil {
			return nil, err
		}
		spendableVtxos = append(spendableVtxos, page...)
		if next == "" {
			break
		}
		cursor = next
	}

	var expiry *time.Time

	if len(spendableVtxos) > 0 {
		for _, vtxo := range spendableVtxos[:] {
			if vtxo.ExpiresAt.Before(time.Now()) {
				return &vtxo.ExpiresAt, nil
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
		if len(tx.BoardingTxid) > 0 && tx.SettledBy == "" {
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

// refreshSettlementSchedule recomputes the next settlement time from the full
// current vtxo set and (re)schedules it. If some vtxos are already expired it
// settles immediately to renew them. It must be used instead of scheduling off
// the delta of a single event, so that vtxos already held by the wallet (e.g.
// left over by a previous batch) cannot expire unnoticed behind a later
// scheduled settlement.
func (s *Service) refreshSettlementSchedule(ctx context.Context, data *clientTypes.Config) error {
	nextExpiry, err := s.computeNextExpiry(ctx, data)
	if err != nil {
		return err
	}
	if nextExpiry == nil {
		return nil
	}

	// If the next expiry is in the past, settle immediately because some vtxos
	// expired. The renewal runs in the background (single-flighted) so it does
	// not block the caller (e.g. the vtxo event loop); the resulting vtxo events
	// will reschedule the next settlement.
	if nextExpiry.Before(time.Now()) {
		s.renewExpiredVtxos(ctx, data)
		return nil
	}

	return s.scheduleNextSettlement(*nextExpiry, data)
}

// renewExpiredVtxos settles in the background to renew already-expired vtxos.
// It is single-flighted: if a renewal is already running, the call is a no-op,
// so a burst of vtxo events cannot pile up redundant settlements.
func (s *Service) renewExpiredVtxos(ctx context.Context, data *clientTypes.Config) {
	if !s.renewing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		log.Debug("detected expired vtxos, joining a batch to renew them...")
		// Use the guarded Settle so we never settle while the node is locked
		// (the renewal can be detected just before a Lock and run afterwards).
		_, err := s.Settle(ctx)

		// Release the single-flight guard before recomputing: if more vtxos
		// expired while we were settling (e.g. a capped batch left some behind),
		// the follow-up refresh can renew them right away instead of waiting for
		// the periodic safety ticker.
		s.renewing.Store(false)

		if err != nil {
			log.WithError(err).Error("failed to renew expired vtxos")
			return
		}

		// Recompute from the full set in case more vtxos are still near expiry.
		if err := s.refreshSettlementSchedule(ctx, data); err != nil {
			log.WithError(err).Error("failed to reschedule after renewing expired vtxos")
		}
	}()
}

func (s *Service) scheduleNextSettlement(at time.Time, data *clientTypes.Config) error {
	task := func() {
		if _, err := s.Settle(context.Background()); err != nil {
			log.WithError(err).Warn("failed to renew vtxos")
		}
		// Recompute the next settlement from the full vtxo set after settling.
		// This way any near-expiry vtxo that was not part of the batch is not
		// left stranded, and a failed settle is retried instead of silently
		// stopping the auto-settlement loop.
		if err := s.refreshSettlementSchedule(context.Background(), data); err != nil {
			log.WithError(err).Error("failed to reschedule settlement after renewing vtxos")
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
	if !nextSettlement.IsZero() && at.After(now) && at.After(nextSettlement) {
		log.Debugf(
			"scheduling next settlement at %s skipped - one already set at %s",
			at.Format(time.RFC3339), nextSettlement.Format(time.RFC3339),
		)
		return nil
	}

	if err := s.schedulerSvc.ScheduleNextSettlement(at, task); err != nil {
		return err
	}
	log.Infof("scheduled next settlement at %s", at.Format(time.RFC3339))
	return nil
}

// vtxoExpiryCheckInterval is how often the vtxo event listener recomputes the
// next settlement from the full vtxo set, as a safety net in case a vtxo event
// is missed (the sdk drops events when its buffer is congested).
const vtxoExpiryCheckInterval = 10 * time.Minute

// subscribeForVtxoEvent keeps the scheduled settlement in sync with the wallet's
// vtxo set: whenever vtxos are added or spent, and periodically as a safety net,
// it recomputes the earliest expiry across all spendable vtxos and reschedules
// the next settlement (settling immediately if anything already expired).
func (s *Service) subscribeForVtxoEvent(ctx context.Context, cfg *clientTypes.Config) {
	eventsCh := s.GetVtxoEventChannel(ctx)

	ticker := time.NewTicker(vtxoExpiryCheckInterval)
	defer ticker.Stop()

	refresh := func() {
		if err := s.refreshSettlementSchedule(ctx, cfg); err != nil {
			// Do not stop the listener on error: a transient failure must not
			// permanently disable auto-settlement. The next event or tick retries.
			log.WithError(err).Error("failed to refresh settlement schedule")
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		case event, ok := <-eventsCh:
			if !ok {
				return
			}

			// Only adding or spending vtxos can change the earliest expiry.
			if event.Type != types.VtxosAdded && event.Type != types.VtxosSpent {
				continue
			}
			if len(event.Vtxos) == 0 {
				continue
			}

			refresh()
		}
	}
}

// handleAddressEventChannel is used to forward address events to the notifications channel
func (s *Service) handleAddressEventChannel(
	config *clientTypes.Config,
) func(event indexer.ScriptEvent) {
	return func(event indexer.ScriptEvent) {
		if event.Connection != nil {
			return
		}
		if event.Err != nil {
			log.WithError(event.Err).Errorf("%s received unexpected error", logPrefix)
			return
		}

		data := event.Data
		if len(data.SpentVtxos) <= 0 && len(data.NewVtxos) <= 0 {
			log.Warnf("%s received unexpected empty event", logPrefix)
			return
		}

		// convert scripts to addresses
		addresses := make([]string, 0, len(data.Scripts))
		for _, script := range data.Scripts {
			decodedPubKey, err := hex.DecodeString(script)
			if err != nil {
				log.WithError(err).Errorf("%s failed to decode script %s", logPrefix, script)
				continue
			}
			vtxoTapPubkey, err := schnorr.ParsePubKey(decodedPubKey[2:])
			if err != nil {
				log.WithError(err).Errorf("%s failed to parse pubkey %s", logPrefix, script)
				continue
			}

			vtxoAddress := arklib.Address{
				VtxoTapKey: vtxoTapPubkey,
				Signer:     config.SignerPubKey,
				HRP:        config.Network.Addr,
			}

			encodedAddress, err := vtxoAddress.EncodeV0()
			if err != nil {
				log.WithError(err).Errorf("%s failed to encode address %s", logPrefix, script)
				continue
			}
			addresses = append(addresses, encodedAddress)

		}

		type logData struct {
			Txid          string
			Addresses     []string
			NewVtxos      int
			SpentVtxos    int
			CheckpointTxs []string
		}
		log.WithField("event", logData{
			Txid:          data.Txid,
			Addresses:     addresses,
			NewVtxos:      len(data.NewVtxos),
			SpentVtxos:    len(data.SpentVtxos),
			CheckpointTxs: slices.Collect(maps.Keys(data.CheckpointTxs)),
		}).Debugf("%s received event for address(es)", logPrefix)

		go func(evt indexer.ScriptEvent) {
			select {
			case s.notifications <- Notification{
				Addrs:       addresses,
				NewVtxos:    data.NewVtxos,
				SpentVtxos:  data.SpentVtxos,
				Checkpoints: data.CheckpointTxs,
				TxData:      indexer.TxData{Tx: data.Tx, Txid: data.Txid},
			}:
				log.Debugf("%s forwarded notification", logPrefix)
			default:
				log.Warnf("%s failed to forward notification", logPrefix)
			}
		}(event)
	}
}

func (s *Service) scheduleSwapRefund(swapId string, opts vhtlc.Opts) (err error) {
	unilateral := func() {
		vtxos, err := s.swapHandler.GetVHTLCFunds(context.Background(), []vhtlc.Opts{opts})
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

		txid, err := s.swapHandler.RefundSwap(
			context.Background(), swap.SwapTypeSubmarine, swapId, false, opts, nil,
		)
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
		if err := s.schedulerSvc.ScheduleTaskAtTime(at, unilateral); err != nil {
			return err
		}
		log.Debugf("scheduled unilateral refund of swap %s at %s", swapId, at.Format(time.RFC3339))
	} else {
		if err := s.schedulerSvc.ScheduleTaskAtHeight(uint32(refundLT), unilateral); err != nil {
			return err
		}
		log.Debugf("scheduling vhtlc refund of swap %s at height %d", swapId, int64(refundLT))
	}

	return err
}

func (s *Service) scheduleChainSwapRefund(swapId string, opts vhtlc.Opts) (err error) {
	unilateral := func() {
		chainSwap, err := s.dbSvc.ChainSwaps().Get(context.Background(), swapId)
		if err != nil {
			log.WithError(err).Errorf("failed to get chain swap %s", swapId)
			return
		}

		vtxos, err := s.swapHandler.GetVHTLCFunds(context.Background(), []vhtlc.Opts{opts})
		if err != nil {
			log.WithError(err).Error("failed to check vhtlc status")
			return
		}
		if len(vtxos) == 0 {
			log.WithError(err).Errorf("vhtlc %s not found", opts.PreimageHash)
			return
		}

		if vtxos[0].Spent {
			errMsg := fmt.Sprintf(
				"cannot refund chain swap %v: VTXO already spent (may have been claimed)",
				chainSwap.Id,
			)
			log.Warn(errMsg)

			chainSwap.Failed(errMsg)

			if err := s.dbSvc.ChainSwaps().Update(context.Background(), *chainSwap); err != nil {
				log.WithError(err).Error("failed to update chain swap data to db")
			}
			return
		}

		txid, err := s.swapHandler.RefundSwap(
			context.Background(), swap.SwapTypeChain, swapId, false, opts, nil,
		)
		if err != nil {
			log.WithError(err).Error("failed to refund chain swap vhtlc")
			return
		}

		chainSwap.RefundedUnilaterally(txid)

		if err := s.dbSvc.ChainSwaps().Update(context.Background(), *chainSwap); err != nil {
			log.WithError(err).Error("failed to update chain swap data to db")
		}

		log.Infof("chain swap vhtlc refunded unilaterally %s", txid)
	}

	refundLT := opts.RefundLocktime

	if refundLT.IsSeconds() {
		at := time.Unix(int64(refundLT), 0)
		if err := s.schedulerSvc.ScheduleTaskAtTime(at, unilateral); err != nil {
			return err
		}
		log.Debugf(
			"scheduled unilateral refund of chain swap %s at %s", swapId, at.Format(time.RFC3339),
		)
	} else {
		if err := s.schedulerSvc.ScheduleTaskAtHeight(uint32(refundLT), unilateral); err != nil {
			return err
		}
		log.Debugf(
			"scheduling chain swap vhtlc refund of swap %s at height %d", swapId, int64(refundLT),
		)
	}

	return err
}

func (s *Service) resumePendingSwapRefunds(ctx context.Context) {
	swaps, err := s.dbSvc.Swap().GetAll(ctx)
	if err != nil {
		log.WithError(err).Error("failed to load swaps while rescheduling refunds")
		return
	}

	for _, swap := range swaps {

		if swap.Status == domain.SwapFailed && swap.RedeemTxId == "" &&
			swap.From == boltz.CurrencyArk {
			if err := s.scheduleSwapRefund(swap.Id, swap.Vhtlc.Opts); err != nil {
				log.WithError(err).WithField("swap_id", swap.Id).Warn(
					"failed to reschedule refund task",
				)
			}
		}

	}
}

func (s *Service) recoverChainSwaps(ctx context.Context, arkConfig *clientTypes.Config) {
	if s.swapHandler == nil {
		log.Warn("swap handler not initialized, skipping chain swap recovery")
		return
	}
	if arkConfig == nil {
		log.Warn("missing ark config, skipping chain swap recovery")
		return
	}

	swaps, err := s.dbSvc.ChainSwaps().GetAll(ctx)
	if err != nil {
		log.WithError(err).Error("failed to load chain swaps for recovery")
		return
	}
	if len(swaps) == 0 {
		return
	}

	network := networkNameToParams(arkConfig.Network.Name)

	eventCallback := func(event swap.ChainSwapEvent) {
		s.handleChainSwapEvent(context.Background(), event)
	}

	unilateralRefund := func(swapId string, opts vhtlc.Opts) error {
		return s.scheduleChainSwapRefund(swapId, opts)
	}

	for _, chainSwap := range swaps {
		if domain.ShouldRefundChainSwapStatus(chainSwap.Status) {
			swapID := chainSwap.Id
			go func() {
				if err := s.RefundChainSwap(context.Background(), swapID); err != nil {
					log.WithError(err).Warnf("failed to recover refund for chain swap %s", swapID)
				}
			}()
			continue
		}

		if domain.ShouldResumeChainSwapStatus(chainSwap.Status) {
			if err := s.resumeChainSwapMonitoring(
				context.Background(),
				chainSwap,
				network,
				eventCallback,
				unilateralRefund,
			); err != nil {
				log.WithError(err).Warnf("failed to resume chain swap %s", chainSwap.Id)
			}
		}
	}
}

func (s *Service) resumeChainSwapMonitoring(
	ctx context.Context,
	chainSwap domain.ChainSwap,
	network *chaincfg.Params,
	eventCallback swap.ChainSwapEventCallback,
	unilateralRefund func(swapId string, opts vhtlc.Opts) error,
) error {
	if chainSwap.BoltzCreateResponseJSON == "" {
		return fmt.Errorf("missing boltz response json")
	}
	if chainSwap.ClaimPreimage == "" {
		return fmt.Errorf("missing preimage")
	}

	_, err := s.swapHandler.ResumeChainSwap(ctx, swap.ResumeChainSwapParams{
		SwapID:             chainSwap.Id,
		From:               chainSwap.From,
		To:                 chainSwap.To,
		Amount:             chainSwap.Amount,
		PreimageHex:        chainSwap.ClaimPreimage,
		BoltzResponseJSON:  chainSwap.BoltzCreateResponseJSON,
		UserBtcAddress:     chainSwap.UserBtcLockupAddress,
		UserLockTxid:       chainSwap.UserLockupTxId,
		ServerLockTxid:     chainSwap.ServerLockupTxId,
		ClaimTxid:          chainSwap.ClaimTxId,
		RefundTxid:         chainSwap.RefundTxId,
		Status:             swap.ChainSwapStatus(chainSwap.Status),
		Error:              chainSwap.ErrorMessage,
		Timestamp:          chainSwap.CreatedAt,
		Network:            network,
		EventCallback:      eventCallback,
		UnilateralRefundCB: unilateralRefund,
	})
	return err
}

// sanitize removes stale boarding UTXOs from the local DB that no longer
// exist on-chain.
func (s *Service) sanitize(ctx context.Context) {
	boardingAddr, err := s.NewBoardingAddress(ctx)
	if err != nil {
		log.WithError(err).Warn("sanitize: failed to get boarding addresses")
		return
	}

	utxoStore := s.Store().UtxoStore()
	spendable, _, err := utxoStore.GetAllUtxos(ctx)
	if err != nil {
		log.WithError(err).Warn("sanitize: failed to get stored utxos")
		return
	}
	if len(spendable) == 0 {
		return
	}

	// Collect all on-chain UTXOs across all boarding addresses.
	onchainUtxos := make(map[string]struct{})
	explorerUtxos, err := s.Explorer().GetUtxos([]string{boardingAddr})
	if err != nil {
		log.WithError(err).Warnf("sanitize: failed to get utxos for %s", boardingAddr)
		return
	}
	for _, u := range explorerUtxos {
		key := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
		onchainUtxos[key] = struct{}{}
	}

	// Find stored UTXOs that are not on-chain and delete them.
	staleOutpoints := make([]clientTypes.Outpoint, 0)
	for _, utxo := range spendable {
		key := fmt.Sprintf("%s:%d", utxo.Txid, utxo.VOut)
		if _, exists := onchainUtxos[key]; !exists {
			staleOutpoints = append(staleOutpoints, utxo.Outpoint)
		}
	}

	if len(staleOutpoints) == 0 {
		return
	}

	count, err := utxoStore.DeleteUtxos(ctx, staleOutpoints)
	if err != nil {
		log.WithError(err).Warn("sanitize: failed to delete stale utxos")
		return
	}
	if count > 0 {
		log.Infof("sanitize: deleted %d stale boarding utxo(s)", count)
	}
}

func convertSwapStatus(swapStatus string) domain.SwapStatus {
	mappedStatus := boltz.ParseEvent(swapStatus)
	if mappedStatus == boltz.TransactionClaimed || mappedStatus == boltz.InvoiceSettled {
		return domain.SwapSuccess
	}

	if mappedStatus == boltz.TransactionClaimPending || mappedStatus == boltz.InvoicePending {
		return domain.SwapPending
	}

	return domain.SwapFailed

}
