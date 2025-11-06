package e2e

import (
	"context"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/test/e2e/setup/nigiri"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	indexer "github.com/arkade-os/go-sdk/indexer"
	indexerTransport "github.com/arkade-os/go-sdk/indexer/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

const (
	syncTimeout         = 2 * time.Minute
	serverURL           = "http://localhost:7070"
	explorerURL         = "http://localhost:3000"
	boltzAPIURL         = "http://localhost:9001"
	boltzWSURL          = "ws://localhost:9004"
	defaultPassword     = "secret"
	fundingAmountBTC    = "0.001"
	swapTimeoutSeconds  = 120
	fundingTimeout      = 5 * time.Minute
	minOffchainExpected = 90_000
	fundingRounds       = 3
)

type swapTestClient struct {
	name      string
	ark       arksdk.ArkClient
	transport client.TransportClient
	indexer   indexer.Indexer
	handler   *swap.SwapHandler
}

func newSwapTestClient(t *testing.T, name string) *swapTestClient {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	storeConfig := store.Config{
		ConfigStoreType:  types.InMemoryStore,
		AppDataStoreType: types.SQLStore,
		BaseDir:          t.TempDir(),
	}

	storeSvc, err := store.NewStore(storeConfig)
	require.NoError(t, err, "%s: create store", name)

	arkClient, err := arksdk.NewArkClient(storeSvc)
	require.NoError(t, err, "%s: create ark client", name)

	privateKey, err := btcec.NewPrivateKey()
	require.NoError(t, err, "%s: generate private key", name)

	seed := hex.EncodeToString(privateKey.Serialize())
	err = arkClient.Init(ctx, arksdk.InitArgs{
		ClientType:           arksdk.GrpcClient,
		WalletType:           arksdk.SingleKeyWallet,
		ServerUrl:            serverURL,
		ExplorerURL:          explorerURL,
		Password:             defaultPassword,
		Seed:                 seed,
		WithTransactionFeed:  true,
		ExplorerPollInterval: 2 * time.Second,
	})
	require.NoError(t, err, "%s: init ark client", name)

	err = arkClient.Unlock(ctx, defaultPassword)
	require.NoError(t, err, "%s: unlock ark client", name)

	waitForSync(t, name, arkClient)

	transportClient, err := grpcclient.NewClient(serverURL)
	require.NoError(t, err, "%s: setup transport client", name)

	indexerClient, err := indexerTransport.NewClient(serverURL)
	require.NoError(t, err, "%s: setup indexer client", name)

	boltzAPI := &boltz.Api{
		URL:   boltzAPIURL,
		WSURL: boltzWSURL,
		Client: http.Client{
			Timeout: 30 * time.Second,
		},
	}

	clientInstance := &swapTestClient{
		name:      name,
		ark:       arkClient,
		transport: transportClient,
		indexer:   indexerClient,
		handler:   swap.NewSwapHandler(arkClient, transportClient, indexerClient, boltzAPI, privateKey.PubKey(), swapTimeoutSeconds),
	}

	t.Cleanup(func() {
		transportClient.Close()
		indexerClient.Close()
		arkClient.Stop()
		storeSvc.Clean(context.Background())
		storeSvc.Close()
	})

	return clientInstance
}

func (c *swapTestClient) ensureFunding(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), fundingTimeout)
	defer cancel()

	for i := 0; i < fundingRounds; i++ {
		_, _, boarding, err := c.ark.Receive(ctx)
		require.NoError(t, err, "%s: get boarding address", c.name)
		require.NotEmpty(t, boarding, "%s: empty boarding address", c.name)

		err = nigiri.Faucet(ctx, boarding, fundingAmountBTC)
		require.NoError(t, err, "%s: faucet to %s", c.name, boarding)
	}

	require.NoError(t, nigiri.MineBlocks(ctx, 1), "%s: mine blocks", c.name)
	time.Sleep(11 * time.Second)

	_, err := c.ark.Settle(ctx)
	require.NoError(t, err, "%s: settle funds", c.name)

	err = swap.Retry(ctx, 2*time.Second, func(ctx context.Context) (bool, error) {
		balance, err := c.ark.Balance(ctx, false)
		if err != nil {
			return false, err
		}
		return balance.OffchainBalance.Total >= minOffchainExpected, nil
	})
	require.NoError(t, err, "%s: wait for offchain balance", c.name)
}

func waitForSync(t *testing.T, name string, client arksdk.ArkClient) {
	t.Helper()

	ch := client.IsSynced(context.Background())
	if ch == nil {
		return
	}

	select {
	case event := <-ch:
		require.True(t, event.Synced, "%s: ark client sync failed: %v", name, event.Err)
	case <-time.After(syncTimeout):
		t.Fatalf("%s: timed out waiting for ark client sync", name)
	}
}
