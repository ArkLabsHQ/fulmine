package application

import (
	"context"
	"testing"

	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestUnwindFailedUnlockReLocks is the regression test for in-session recovery:
// when UnlockNode's post-sync setup fails, unwindFailedUnlock must roll the
// wallet back to a locked state (so a fresh unlock can retry) rather than leave
// it stuck not-ready until a restart.
func TestUnwindFailedUnlockReLocks(t *testing.T) {
	fake := newFakeArkClient()
	svc := &Service{
		ArkClient:     fake,
		isInitialized: true,
		syncEvent:     &types.SyncEvent{},
	}
	svc.walletReady.Store(true) // pretend assembly got partway before the failure

	// No listener was started, so vtxoListenerCancel is nil and the rollback must
	// simply skip the listener stop (not deref a nil cancel).
	svc.unwindFailedUnlock()

	require.True(t, fake.wasLocked(), "a failed unlock must re-lock so it can be retried")
	require.False(t, svc.walletReady.Load(), "rollback must clear walletReady")
	require.Nil(t, svc.syncEvent, "rollback must clear syncEvent")
}

// TestUnwindFailedUnlockStopsDelegate guards the lifecycle symmetry: UnlockNode's
// post-sync goroutine starts the delegate service (via onUnlock) before assembling
// the swap handler, so a later failure that triggers unwindFailedUnlock must also
// stop it (via onLock) — otherwise the delegate event loops keep running against a
// wallet that rollback just re-locked.
func TestUnwindFailedUnlockStopsDelegate(t *testing.T) {
	fake := newFakeArkClient()
	svc := &Service{
		ArkClient:     fake,
		isInitialized: true,
		syncEvent:     &types.SyncEvent{},
	}
	svc.walletReady.Store(true)

	// Wire the delegate lifecycle the way newServiceWithDelegate does.
	delegateSvc := newDelegateService(svc, 0)
	svc.onUnlock = func() { delegateSvc.start() }
	svc.onLock = func() { delegateSvc.Stop() }

	// Simulate onUnlock already having started the delegate (the state start()
	// leaves behind) without launching its background goroutines.
	delegateCtx, cancel := context.WithCancel(context.Background())
	delegateSvc.ctx = delegateCtx
	delegateSvc.cancelFunc = cancel

	svc.unwindFailedUnlock()

	require.Nil(t, delegateSvc.cancelFunc, "rollback must stop the delegate service")
	require.Error(t, delegateCtx.Err(), "rollback must cancel the delegate context so its loops exit")
}
