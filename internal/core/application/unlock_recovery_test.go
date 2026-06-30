package application

import (
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
