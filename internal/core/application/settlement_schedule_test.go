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
	eventCh    chan types.VtxoEvent
	settles    int
	lockCalled bool
	// onSettle, if set, is run while holding the lock when Settle is called,
	// so a test can simulate the vtxo set being renewed by the settlement.
	onSettle func()
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

func (f *fakeArkClient) ListVtxos(_ context.Context) (spendable, spent []clientTypes.Vtxo, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]clientTypes.Vtxo, len(f.spendable))
	copy(out, f.spendable)
	return out, nil, nil
}

func (f *fakeArkClient) GetTransactionHistory(_ context.Context) ([]clientTypes.Transaction, error) {
	return nil, nil
}

func (f *fakeArkClient) GetVtxoEventChannel(_ context.Context) <-chan types.VtxoEvent {
	return f.eventCh
}

func (f *fakeArkClient) IsLocked(_ context.Context) bool { return false }

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
