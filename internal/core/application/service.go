package application

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	client "github.com/arkade-os/arkd/pkg/client-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/identity"
	singlekeyidentity "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey"
	singlekeyfilestore "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey/store/file"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientstore "github.com/arkade-os/arkd/pkg/client-lib/store"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/contract"
	hdidentity "github.com/arkade-os/go-sdk/identity"
	"github.com/arkade-os/go-sdk/types"
	"github.com/arkade-os/go-sdk/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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
	configStore    clientTypes.ConfigStore
	dbSvc          ports.RepoManager
	schedulerSvc   ports.SchedulerService
	delegateConfig DelegateConfig

	publicKey  *btcec.PublicKey
	privateKey *btcec.PrivateKey

	// emulatorPubKey is the server-configured non-interactive claim tapscript
	// key. Nil when unset, which disables non-interactive claims.
	emulatorPubKey *btcec.PublicKey
	esploraUrl     string

	isInitialized bool
	walletReady   atomic.Bool // true once UnlockNode has done syncing
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

type DelegateConfig struct {
	Enabled bool
	Fee     uint64
}

func NewServices(
	dbSvc ports.RepoManager, schedulerSvc ports.SchedulerService, delegateConfig DelegateConfig,
	buildInfo BuildInfo, datadir, esploraUrl, emulatorPubkeyHex string, refreshDbInterval int64,
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
		dbSvc, schedulerSvc, buildInfo, datadir, esploraUrl, refreshDbInterval, emulatorPubKey,
	)
	if err != nil {
		return nil, nil, err
	}

	svc.delegateConfig = delegateConfig

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

func (s *Service) Setup(ctx context.Context, serverUrl, password, mnemonic string) (err error) {
	if s.isInitialized {
		return errors.New("wallet already initialized")
	}

	if err := utils.IsValidMnemonic(mnemonic); err != nil {
		return err
	}

	validatedServerUrl, err := utils.ValidateURL(serverUrl)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	var opts []arksdk.InitOption
	if s.esploraUrl != "" {
		opts = append(opts, arksdk.WithExplorerURL(s.esploraUrl))
	}

	if err := s.Init(ctx, validatedServerUrl, mnemonic, password, opts...); err != nil {
		return err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	s.esploraUrl = config.ExplorerURL
	// s.publicKey = prvKey.PubKey()
	// s.privateKey = prvKey
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

		if s.delegateConfig.Enabled {
			// Load delegate signer key.
			mnemonic, err := s.Dump(finalizeCtx)
			if err != nil {
				log.WithError(err).Error("failed to get delegate signer key")
				s.unwindFailedUnlock()
				return
			}

			privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic, arkConfig.Network.Name)
			if err != nil {
				s.unwindFailedUnlock()
				log.WithError(err).Error("failed to decode delegate signer key")
				return
			}

			s.publicKey = privateKey.PubKey()
			s.privateKey = privateKey
		}

		if s.onUnlock != nil {
			s.onUnlock()
		}

		// Backward compat: a single-key wallet upgrading from v3 has its vhtlcs only
		// as parameter rows in vhtlc_legacy. Rebuild + register them before the wallet
		// is advertised as ready.
		s.migrateVhtlcs(finalizeCtx)

		// All gate-required fields are populated; open the gate. The atomic store
		// publishes the writes above to any reader that passes the gate.
		s.walletReady.Store(true)

		s.sanitize(context.Background())
	}()

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletUnlock, Password: password}
	}()

	return nil
}

func (s *Service) ResetWallet(ctx context.Context) error {
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

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletReset}
	}()
	return nil
}

func (s *Service) GetPubkey(ctx context.Context) (*btcec.PublicKey, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	identitySvc := s.Identity()
	keyId := ""
	if identitySvc.GetType() == hdidentity.Type {
		keyId = "0/0"
	}
	keyRef, err := identitySvc.GetKey(ctx, keyId)
	if err != nil {
		return nil, err
	}
	return keyRef.PubKey, nil
}

func (s *Service) NewAddress(
	ctx context.Context, sats uint64,
) (bip21Addr, offchainAddr, boardingAddr string, err error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", "", err
	}

	boardingAddr, err = s.NewBoardingAddress(ctx)
	if err != nil {
		return "", "", "", err
	}
	offchainAddr, err = s.NewOffchainAddress(ctx)
	if err != nil {
		return "", "", "", err
	}

	bip21Addr = fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)

	if sats == 0 {
		return
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

func (s *Service) GetVHTLCSpendingTx(ctx context.Context, vhtlcId string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	record, err := s.dbSvc.VHTLC().Get(ctx, vhtlcId)
	if err != nil {
		return "", fmt.Errorf("failed to get VHTLC %s: %w", vhtlcId, err)
	}

	return s.getVHTLCSpendingTx(ctx, *record)
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

func (s *Service) WhenNextRenewal(ctx context.Context) time.Time {
	return s.Wallet.WhenNextRenewal()
}

func (s *Service) CreateVHTLC(
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

	var nonInteractiveReceiver []byte
	var nonInteractiveEmulator *btcec.PublicKey
	if nonInteractiveClaimAddress != nil {
		if s.emulatorPubKey == nil {
			return "", "", nil, fmt.Errorf("non-interactive claims are disabled: missing EMULATOR_PUBKEY")
		}
		if nonInteractiveClaimAddress.HRP != cfg.Network.Addr {
			return "", "", nil, fmt.Errorf("non-interactive claim address has wrong network")
		}

		pkScript, err := nonInteractiveClaimAddress.GetPkScript()
		if err != nil {
			return "", "", nil, fmt.Errorf("invalid non-interactive claim address")
		}

		nonInteractiveReceiver = pkScript
		nonInteractiveEmulator = s.emulatorPubKey
	}

	vhtlcScript, err := s.Wallet.CreateVHTLC(ctx, contract.VHTLCContractArgs{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
		NonInteractiveReceiver:               nonInteractiveReceiver,
		NonInteractiveEmulator:               nonInteractiveEmulator,
	})
	if err != nil {
		return "", "", nil, err
	}

	encodedAddr, err := vhtlcScript.Address(cfg.Network.Addr)
	if err != nil {
		return "", "", nil, err
	}

	pkScript, err := vhtlcScript.PkScript()
	if err != nil {
		return "", "", nil, err
	}
	script := hex.EncodeToString(pkScript)

	compressedReceiverPubkey := vhtlcScript.Receiver.SerializeCompressed()
	compressedSenderPubkey := vhtlcScript.Sender.SerializeCompressed()
	vhtlcId := domain.GetVhtlcId(preimageHash, compressedSenderPubkey, compressedReceiverPubkey)

	// Persist synchronously: the duplicate check above (VHTLC().Get) and callers
	// like ListVHTLC read the record back immediately, so adding it in a detached
	// goroutine raced the read — intermittently letting duplicates through and
	// making the record briefly invisible (flaky e2e: TestVHTLC dedup and
	// TestSettleVHTLCByDelegateRefundWithOutpoint).
	if err := s.dbSvc.VHTLC().Add(
		ctx, domain.NewVhtlc(vhtlcId, script)); err != nil {
		return "", "", nil, fmt.Errorf("failed to add vhtlc: %w", err)
	}
	log.Debugf("added new vhtlc %s", vhtlcId)

	return encodedAddr, vhtlcId, vhtlcScript, nil
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

func (s *Service) ListVHTLCs(ctx context.Context, vhtlcIds []string) ([]clientTypes.Vtxo, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	// Return empty list if an empty one is provided
	if len(vhtlcIds) <= 0 {
		return nil, nil
	}

	records, err := s.dbSvc.VHTLC().GetByIds(ctx, vhtlcIds)
	if err != nil {
		return nil, err
	}

	return s.getVHTLCsFunds(ctx, records)
}

func (s *Service) ClaimVHTLC(
	ctx context.Context, preimage []byte, vhtlc_id string, outpoint *clientTypes.Outpoint,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	var opts []arksdk.VHTLCOption
	if outpoint != nil {
		opts = append(opts, arksdk.WithOutpoint(*outpoint))
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	return s.Wallet.ClaimVHTLC(ctx, vhtlc.Script, preimage, opts...)
}

func (s *Service) RefundVHTLC(
	ctx context.Context, swapId, vhtlc_id string, withReceiver bool, outpoint *clientTypes.Outpoint,
) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	var opts []arksdk.VHTLCOption
	if outpoint != nil {
		opts = append(opts, arksdk.WithOutpoint(*outpoint))
	}

	vhtlc, err := s.dbSvc.VHTLC().Get(ctx, vhtlc_id)
	if err != nil {
		return "", err
	}

	return s.UnilateralRefundVHTLC(ctx, vhtlc.Script, opts...)
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

	if !s.walletReady.Load() {
		return fmt.Errorf("wallet is finalizing unlock")
	}

	return nil
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

// sanitize removes stale boarding UTXOs from the local DB that no longer
// exist on-chain (they've been RBF-ed).
// TODO: Drop me and handle this in go-sdk.
func (s *Service) sanitize(ctx context.Context) {
	_, _, boardingAddrs, _, err := s.GetAddresses(ctx)
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
	explorerUtxos, err := s.Explorer().GetUtxos(boardingAddrs)
	if err != nil {
		log.WithError(err).Warn("sanitize: failed to get utxos for boarding addresses")
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

func (s *Service) getVHTLCSpendingTx(ctx context.Context, vhtlc domain.Vhtlc) (string, error) {
	vhtlcs, err := s.Wallet.ListVHTLCs(ctx, arksdk.WithScripts([]string{vhtlc.Script}))
	if err != nil {
		return "", err
	}
	if len(vhtlcs) <= 0 {
		return "", fmt.Errorf(
			"no contract found for vhtlc %s with script %s", vhtlc.Id, vhtlc.Script,
		)
	}
	vhtlcScript := vhtlcs[0]

	vtxos, err := s.getVHTLCsFunds(ctx, []domain.Vhtlc{vhtlc})
	if err != nil {
		return "", err
	}
	if len(vtxos) == 0 {
		return "", nil
	}
	vtxo := vtxos[0]

	isPending := vtxo.SpentBy != "" && vtxo.SettledBy == "" && vtxo.ArkTxid == ""
	if isPending {
		tx, err := s.getPendingVHTLCTx(ctx, vtxo, vhtlcScript)
		if err != nil {
			return "", fmt.Errorf("failed to get pending tx: %w", err)
		}
		return tx, nil
	}

	txs, err := s.Wallet.Indexer().GetVirtualTxs(ctx, []string{vtxo.ArkTxid})
	if err != nil {
		return "", fmt.Errorf("failed to get virtual tx: %w", err)
	}
	if len(txs.Txs) == 0 {
		return "", fmt.Errorf("no virtual tx found for txid %s", vtxo.ArkTxid)
	}

	return txs.Txs[0], nil
}

func (s *Service) getVHTLCsFunds(
	ctx context.Context, vhtlcs []domain.Vhtlc,
) ([]clientTypes.Vtxo, error) {
	scripts := make([]string, 0, len(vhtlcs))
	for _, v := range vhtlcs {
		scripts = append(scripts, v.Script)
	}
	resp, err := s.Wallet.Indexer().GetVtxos(ctx, indexer.WithScripts(scripts))
	if err != nil {
		return nil, err
	}
	return resp.Vtxos, nil
}

func (s *Service) getPendingVHTLCTx(
	ctx context.Context, vtxo clientTypes.Vtxo, vhtlcScript vhtlc.VHTLCScript,
) (string, error) {
	inputs := []pendingTxIntentInput{{
		Vtxo: clientTypes.VtxoWithTapTree{
			Vtxo:       vtxo,
			Tapscripts: vhtlcScript.GetRevealedTapscripts(),
		},
		Closure:  vhtlcScript.RefundWithoutReceiverClosure,
		Sequence: wire.MaxTxInSequenceNum - 1,
	}}

	proof, message, err := getPendingTxIntent(inputs, uint32(vhtlcScript.RefundWithoutReceiverClosure.Locktime))
	if err != nil {
		return "", err
	}

	signedProof, err := s.SignTransaction(ctx, proof)
	if err != nil {
		return "", fmt.Errorf("failed to sign pending tx proof: %w", err)
	}

	pendingTxs, err := s.Wallet.Client().GetPendingTx(ctx, signedProof, message)
	if err != nil {
		return "", err
	}

	if len(pendingTxs) == 0 {
		return "", fmt.Errorf("no pending txs found")
	}

	return pendingTxs[0].FinalArkTx, nil
}

func newService(
	dbSvc ports.RepoManager, schedulerSvc ports.SchedulerService, buildInfo BuildInfo,
	datadir, esploraUrl string, refreshDbInterval int64, emulatorPubKey *btcec.PublicKey,
) (*Service, error) {
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
	}

	// Backward compatibility: try to load the single key identity used by v0.3 and previous
	// versions, otherwise load or create a new wallet with the default one (HD).
	singleKeyIdentity, err := loadSingleKeyIdentity(datadir)
	if err != nil {
		return nil, err
	}
	if singleKeyIdentity != nil {
		opts = append(opts, arksdk.WithIdentity(singleKeyIdentity))
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
			BuildInfo:      buildInfo,
			Wallet:         arkClient,
			configStore:    configStore,
			dbSvc:          dbSvc,
			schedulerSvc:   schedulerSvc,
			publicKey:      nil,
			emulatorPubKey: emulatorPubKey,
			isInitialized:  true,
			notifications:  make(chan Notification),
			esploraUrl:     data.ExplorerURL,
			walletUpdates:  make(chan WalletUpdate),
			syncLock:       &sync.RWMutex{},
		}

		if err := svc.RefreshServerConfig(context.Background()); err != nil {
			return nil, err
		}

		return svc, nil
	} else if !strings.Contains(err.Error(), "not initialized") {
		return nil, err
	}

	arkClient, err := arksdk.NewWallet(datadir, opts...)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		BuildInfo:      buildInfo,
		Wallet:         arkClient,
		configStore:    configStore,
		dbSvc:          dbSvc,
		schedulerSvc:   schedulerSvc,
		notifications:  make(chan Notification),
		esploraUrl:     esploraUrl,
		emulatorPubKey: emulatorPubKey,
		walletUpdates:  make(chan WalletUpdate),
		syncLock:       &sync.RWMutex{},
	}

	return svc, nil
}

func loadSingleKeyIdentity(datadir string) (identity.Identity, error) {
	identityStore, err := singlekeyfilestore.NewStore(datadir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet store: %w", err)
	}

	data, err := identityStore.Get()
	if err != nil || data == nil {
		return nil, nil
	}
	return singlekeyidentity.NewIdentity(identityStore)
}
