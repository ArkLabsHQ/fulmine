package application

import (
	"context"
	"testing"

	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestUnlockWindowGateBlocksUntilReady is the regression test for the post-unlock
// nil-deref panic: isInitializedAndUnlocked opened as soon as syncEvent was set,
// but UnlockNode populates publicKey/privateKey/swapHandler later, in its post-sync
// goroutine. A request landing in that window dereferenced a nil field and panicked
// the daemon. The walletReady gate must reject such a request cleanly instead.
func TestUnlockWindowGateBlocksUntilReady(t *testing.T) {
	svc := &Service{
		ArkClient:     newFakeArkClient(), // IsLocked() == false
		isInitialized: true,
		syncEvent:     &types.SyncEvent{}, // sync finished, so the old gate would open...
		// ...but walletReady is still false: the post-sync goroutine hasn't set
		// publicKey/privateKey/swapHandler yet. This is the crash window.
	}

	require.ErrorContains(t, svc.isInitializedAndUnlocked(context.Background()), "finalizing",
		"a request in the unlock window must be rejected, not allowed through to a nil field")

	// Once the post-sync goroutine finishes assembling the wallet, the gate opens.
	svc.walletReady.Store(true)
	require.NoError(t, svc.isInitializedAndUnlocked(context.Background()))
}
