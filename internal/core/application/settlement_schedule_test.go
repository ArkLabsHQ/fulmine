package application

import (
	"context"
	"sync"
	"testing"
	"time"

	scheduler "github.com/ArkLabsHQ/fulmine/internal/infrastructure/scheduler/gocron"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestSettlementScheduleTracksEarliestExpiry is the regression test for the bug
// where the next settlement was computed only from the vtxos contained in a
// single event (and ratcheted earlier-only). A vtxo already held by the wallet
// but absent from the latest event would then expire behind a later scheduled
// settlement. The fix recomputes the schedule from the full vtxo set, so the
// schedule must always reflect the earliest expiry across all spendable vtxos.
func TestSettlementScheduleTracksEarliestExpiry(t *testing.T) {
	fake := newFakeArkClient()
	svc, emit := newTestService(t, fake)

	// 1. A single far-future vtxo arrives -> schedule far in the future.
	far := vtxo("a", 240*time.Hour)
	fake.setVtxos(far)
	emit(types.VtxosAdded, far)

	require.Eventually(t, func() bool {
		next := svc.schedulerSvc.WhenNextSettlement()
		return !next.IsZero() && next.After(time.Now().Add(239*time.Hour))
	}, 2*time.Second, 10*time.Millisecond, "expected a far-future settlement to be scheduled")

	// 2. A nearer vtxo is now also held by the wallet (e.g. left over by a
	//    previous batch / received earlier), but the event that fires mentions
	//    only an unrelated, even-later vtxo. The buggy code looked only at the
	//    event's vtxos and would leave the schedule far in the future, stranding
	//    the nearer vtxo. The fix must pull the schedule down to the nearer one.
	near := vtxo("b", 1*time.Hour)
	later := vtxo("c", 480*time.Hour)
	fake.setVtxos(far, near, later)
	emit(types.VtxosAdded, later)

	require.Eventually(t, func() bool {
		next := svc.schedulerSvc.WhenNextSettlement()
		return !next.IsZero() && next.Before(time.Now().Add(2*time.Hour))
	}, 2*time.Second, 10*time.Millisecond,
		"next settlement should track the earliest-expiring vtxo, not the event's vtxos")

	require.Zero(t, fake.settleCount(), "no vtxo expired, so no immediate settlement should happen")
}

// TestSettlementScheduleSettlesAlreadyExpiredVtxos verifies that when the wallet
// already holds an expired vtxo, the listener renews it immediately (rather than
// scheduling a settlement in the past, which previously errored and killed the
// listener), and then reschedules off the renewed set.
func TestSettlementScheduleSettlesAlreadyExpiredVtxos(t *testing.T) {
	fake := newFakeArkClient()

	// Simulate the renewal: once settled, the expired vtxo is replaced by a
	// fresh far-future one, as a real settlement would do.
	fake.onSettle = func() {
		fake.spendable = []clientTypes.Vtxo{vtxo("renewed", 240*time.Hour)}
	}

	svc, emit := newTestService(t, fake)

	expired := vtxo("old", -1*time.Hour)
	fake.setVtxos(expired)
	emit(types.VtxosAdded, expired)

	// The expired vtxo must trigger exactly one immediate renewal...
	require.Eventually(t, func() bool {
		return fake.settleCount() >= 1
	}, 2*time.Second, 10*time.Millisecond, "expired vtxo should trigger an immediate settlement")

	// ...and then the schedule should follow the renewed, far-future set.
	require.Eventually(t, func() bool {
		next := svc.schedulerSvc.WhenNextSettlement()
		return !next.IsZero() && next.After(time.Now().Add(239*time.Hour))
	}, 2*time.Second, 10*time.Millisecond, "should reschedule off the renewed vtxo set")

	// Single-flight + renewal must keep the renewal from looping.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 1, fake.settleCount(), "renewal must not be repeated in a loop")
}

// fakeArkClient is a minimal stand-in for arksdk.ArkClient that only implements
// the methods exercised by the settlement scheduling logic. Any other method is
// inherited from the (nil) embedded interface and would panic if called, which
// keeps the test honest about what the scheduler actually depends on.
type fakeArkClient struct {
	arksdk.ArkClient

	mu         sync.Mutex
	spendable  []clientTypes.Vtxo
	spent      []clientTypes.Vtxo
	eventCh    chan types.VtxoEvent
	settles    int
	lockCalled bool
	// onSettle, if set, is run while holding the lock when Settle is called,
	// so a test can simulate the vtxo set being renewed by the settlement.
	onSettle func()

	// locked / unlockErr let a test drive UnlockNode's guard and its Unlock call.
	locked    bool
	unlockErr error

	// settleDelay simulates the duration of a real batch session (set before
	// the service starts; read without the lock).
	settleDelay time.Duration
}

func newFakeArkClient() *fakeArkClient {
	return &fakeArkClient{eventCh: make(chan types.VtxoEvent, 16)}
}

func (f *fakeArkClient) setVtxos(vtxos ...clientTypes.Vtxo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spendable = vtxos
}

func (f *fakeArkClient) settleCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settles
}

func (f *fakeArkClient) setSpentVtxos(vtxos ...clientTypes.Vtxo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spent = vtxos
}

func (f *fakeArkClient) ListVtxos(_ context.Context) (spendable, spent []clientTypes.Vtxo, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	spendable = make([]clientTypes.Vtxo, len(f.spendable))
	copy(spendable, f.spendable)
	spent = make([]clientTypes.Vtxo, len(f.spent))
	copy(spent, f.spent)
	return spendable, spent, nil
}

func (f *fakeArkClient) GetTransactionHistory(_ context.Context) ([]clientTypes.Transaction, error) {
	return nil, nil
}

func (f *fakeArkClient) GetVtxoEventChannel(_ context.Context) <-chan types.VtxoEvent {
	return f.eventCh
}

func (f *fakeArkClient) IsLocked(_ context.Context) bool { return f.locked }

func (f *fakeArkClient) Unlock(_ context.Context, _ string) error { return f.unlockErr }

// IsSynced returns a channel that never fires, mimicking the SDK when a wallet
// was never unlocked (e.g. a failed Unlock): the sync never completes.
func (f *fakeArkClient) IsSynced(_ context.Context) <-chan types.SyncEvent {
	return make(chan types.SyncEvent)
}

func (f *fakeArkClient) Lock(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lockCalled = true
	return nil
}

func (f *fakeArkClient) wasLocked() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lockCalled
}

func (f *fakeArkClient) Settle(_ context.Context, _ ...arksdk.BatchSessionOption) (string, error) {
	if f.settleDelay > 0 {
		time.Sleep(f.settleDelay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settles++
	if f.onSettle != nil {
		f.onSettle()
	}
	return "txid", nil
}

func vtxo(txid string, expiresIn time.Duration) clientTypes.Vtxo {
	return clientTypes.Vtxo{
		Outpoint:  clientTypes.Outpoint{Txid: txid, VOut: 0},
		Amount:    1000,
		ExpiresAt: time.Now().Add(expiresIn),
	}
}

// newTestService wires the real gocron scheduler to a Service backed by the fake
// sdk client, and starts the vtxo event listener. It returns the service, the
// fake client, and a function to send vtxo events to the listener.
func newTestService(t *testing.T, fake *fakeArkClient) (*Service, func(types.VtxoEventType, ...clientTypes.Vtxo)) {
	t.Helper()

	sched := scheduler.NewScheduler("", 50*time.Millisecond)
	sched.Start()

	svc := &Service{
		ArkClient:    fake,
		schedulerSvc: sched,
		// Mark the service initialized/unlocked/synced so the guarded Settle
		// (isInitializedAndUnlocked) used by the renewal path is allowed to run.
		isInitialized: true,
		syncEvent:     &types.SyncEvent{},
	}
	// The gate also requires the wallet to be fully assembled (publicKey/swapHandler).
	svc.walletReady.Store(true)

	// SessionDuration is tiny so the 2-session safety offset doesn't push
	// far-future schedules around in a way that would confuse the assertions.
	cfg := &clientTypes.Config{SessionDuration: 1}

	listenerCtx, cancel := context.WithCancel(context.Background())
	svc.vtxoListenerCancel = cancel
	go svc.subscribeForVtxoEvent(listenerCtx, cfg)

	t.Cleanup(func() {
		cancel()
		sched.Stop()
	})

	emit := func(typ types.VtxoEventType, vtxos ...clientTypes.Vtxo) {
		fake.eventCh <- types.VtxoEvent{Type: typ, Vtxos: vtxos}
	}

	return svc, emit
}

// TestSettlementScheduleSingleFlightsDueVtxos is the regression test for the
// duplicate-batch cascade seen in production: a vtxo whose expiry falls inside
// the settle-ahead window (2 session durations) makes the computed settlement
// time land in the past, and every vtxo event then fired an immediate,
// unguarded settle (gocron's delay<=0 branch bypassed the renewing guard).
// Concurrent settles raced each other and arkd rejected the losers with
// VTXO_ALREADY_SPENT. All settle paths must funnel through the single-flight
// guard so a burst of events yields exactly one settlement.
func TestSettlementScheduleSingleFlightsDueVtxos(t *testing.T) {
	fake := newFakeArkClient()
	// A real batch session takes seconds and the events of a completing batch
	// arrive while other refreshes run. Without this delay the fake renews the
	// vtxo set before the event burst is processed and the race never opens.
	fake.settleDelay = 150 * time.Millisecond

	// Once settled, the due vtxo is renewed into a fresh far-future one, as a
	// real settlement would do.
	fake.onSettle = func() {
		fake.spendable = []clientTypes.Vtxo{vtxo("renewed", 240*time.Hour)}
	}

	_, emit := newTestService(t, fake)

	// Expires in 1.5s: not expired yet, but inside the 2s settle-ahead window
	// (SessionDuration is 1s in tests), so the computed settlement time is
	// already in the past.
	due := vtxo("due", 1500*time.Millisecond)
	fake.setVtxos(due)

	// A burst of vtxo events, as emitted by a completing batch.
	for range 4 {
		emit(types.VtxosAdded, due)
	}

	require.Eventually(t, func() bool {
		return fake.settleCount() >= 1
	}, 2*time.Second, 10*time.Millisecond, "a due vtxo should trigger a settlement")

	// The event burst must be absorbed by the single-flight guard: no
	// concurrent or repeated settlements for the same due vtxo.
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 1, fake.settleCount(), "event burst must not fire duplicate settlements")
}

// TestSettlementScheduleIgnoresStaleStoreReads reproduces the sequential
// double-batch: a completed settle updates the store in two steps (the renewed
// vtxo is added before the old ones are marked spent) and each step emits an
// event. A refresh landing between the two steps sees the old vtxo still
// spendable and due, and used to immediately join a second batch that churned
// the freshly renewed vtxo. The renewal must re-verify due-ness on a settled
// store before joining a batch, so the stale trigger evaporates.
func TestSettlementScheduleIgnoresStaleStoreReads(t *testing.T) {
	fake := newFakeArkClient()

	due := vtxo("due", 1500*time.Millisecond)
	renewed := vtxo("renewed", 240*time.Hour)

	// Simulate the sdk's two-step store update: on settle, first the renewed
	// vtxo is added (old one still spendable -> stale window), and only a bit
	// later the old one is marked spent. Each step emits its event.
	fake.onSettle = func() {
		fake.spendable = []clientTypes.Vtxo{due, renewed}
		go func() {
			fake.eventCh <- types.VtxoEvent{Type: types.VtxosAdded, Vtxos: []clientTypes.Vtxo{renewed}}
			time.Sleep(50 * time.Millisecond)
			fake.setVtxos(renewed)
			fake.eventCh <- types.VtxoEvent{Type: types.VtxosSpent, Vtxos: []clientTypes.Vtxo{due}}
		}()
	}

	_, emit := newTestService(t, fake)

	fake.setVtxos(due)
	emit(types.VtxosAdded, due)

	require.Eventually(t, func() bool {
		return fake.settleCount() >= 1
	}, 3*time.Second, 10*time.Millisecond, "the due vtxo should trigger a settlement")

	// Give the stale-window refresh ample time to run a would-be second
	// settlement: the renewed set must not be settled again.
	time.Sleep(1500 * time.Millisecond)
	require.Equal(t, 1, fake.settleCount(), "a mid-update store read must not trigger a second settlement")
}
