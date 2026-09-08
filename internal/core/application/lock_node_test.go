package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLockNodeWhileSyncing(t *testing.T) {
	fake := newLockingFakeArkClient()
	svc := &Service{
		Wallet:        fake,
		isInitialized: true,
		walletUpdates: make(chan WalletUpdate, 1),
	}

	require.NoError(t, svc.LockNode(t.Context()))
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
