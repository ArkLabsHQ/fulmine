package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestLockNodeWhileSyncing(t *testing.T) {
	fake := newLockingFakeArkClient()
	svc := &Service{
		Wallet:        fake,
		isInitialized: true,
		walletUpdates: make(chan WalletUpdate, 1),
		syncLock:      &sync.RWMutex{},
	}

	require.NoError(t, svc.LockNode(t.Context()))
	require.True(t, fake.locked)
}

func TestLockNodeDuringActiveSync(t *testing.T) {
	release := make(chan struct{})
	fake := &syncingFakeArkClient{
		lockingFakeArkClient: newLockingFakeArkClient(),
		syncRelease:          release,
	}
	svc := &Service{
		Wallet:        fake,
		isInitialized: true,
		walletUpdates: make(chan WalletUpdate, 1),
		syncLock:      &sync.RWMutex{},
	}

	svc.syncCh = make(chan types.SyncEvent, 1)
	syncWorkerReady := make(chan struct{})
	go func() {
		svc.syncLock.Lock()
		syncWorkerReady <- struct{}{}
		defer svc.syncLock.Unlock()
		ev := <-fake.IsSynced(context.Background())
		svc.syncEvent = &ev
		svc.syncCh <- ev
	}()
	<-syncWorkerReady

	done := make(chan error, 1)
	go func() {
		done <- svc.LockNode(t.Context())
	}()

	select {
	case err := <-done:
		t.Fatalf("LockNode returned before sync worker finished: %v", err)
	case <-time.After(2 * time.Second):
	}

	close(release)
	require.NoError(t, <-done)
	require.True(t, fake.locked)
}

func TestLockNodeWaitsForUnlockFinalization(t *testing.T) {
	fake := newLockingFakeArkClient()
	svc := &Service{
		Wallet:        fake,
		isInitialized: true,
		walletUpdates: make(chan WalletUpdate, 1),
		syncLock:      &sync.RWMutex{},
	}

	svc.lifecycleLock.Lock()
	done := make(chan error, 1)
	go func() {
		done <- svc.LockNode(t.Context())
	}()

	select {
	case err := <-done:
		t.Fatalf("LockNode returned while unlock finalization was active: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	svc.lifecycleLock.Unlock()
	require.NoError(t, <-done)
	require.True(t, fake.locked)
}

type lockingFakeArkClient struct {
	*fakeArkClient
	locked bool
}

func (f *lockingFakeArkClient) Lock(context.Context) error {
	f.locked = true
	return nil
}

func newLockingFakeArkClient() *lockingFakeArkClient {
	return &lockingFakeArkClient{fakeArkClient: newFakeArkClient()}
}

type syncingFakeArkClient struct {
	*lockingFakeArkClient
	syncRelease chan struct{}
}

func (f *syncingFakeArkClient) IsSynced(ctx context.Context) <-chan types.SyncEvent {
	ch := make(chan types.SyncEvent, 1)
	go func() {
		if f.syncRelease != nil {
			<-f.syncRelease
		}
		ch <- types.SyncEvent{Synced: true}
	}()
	return ch
}
