package e2e

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	indexer "github.com/arkade-os/go-sdk/indexer"
	indexergrpc "github.com/arkade-os/go-sdk/indexer/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

const (
	defaultArkServer  = "http://localhost:7070"
	defaultExplorer   = "http://localhost:3000"
	defaultBoltzHTTP  = "http://localhost:9001"
	defaultBoltzWS    = "ws://localhost:9004"
	defaultHTTPAPI    = "http://localhost:7001/api/v1"
	defaultPassword   = "secret"
	defaultSwapTimout = uint32(30)
	desiredOnchainMin = uint64(150_000)
	arkFaucetAmount   = "0.001"
	arkFaucetRounds   = 3
)

// SwapTest initialises a fresh Ark client and swap handler wired against the
// running integration stack, allowing tests to exercise pkg/swap directly.
type SwapTest struct {
	T          *testing.T
	Ctx        context.Context
	cancel     context.CancelFunc
	Handler    *swap.SwapHandler
	ArkClient  arksdk.ArkClient
	Transport  client.TransportClient
	Indexer    indexer.Indexer
	Boltz      *boltz.Api
	PublicKey  *btcec.PublicKey
	HTTPClient *http.Client
	APIBaseURL string
	Password   string
}

// NewSwapTest prepares the swap harness. Tests must not mutate global Ark or
// Boltz state outside of the resources returned here.
func NewSwapTest(t *testing.T) *SwapTest {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	tempDir := t.TempDir()
	storeSvc, err := store.NewStore(store.Config{
		BaseDir:          tempDir,
		ConfigStoreType:  types.FileStore,
		AppDataStoreType: types.SQLStore,
	})
	require.NoError(t, err, "create store")

	arkClient, err := arksdk.NewArkClient(storeSvc)
	require.NoError(t, err, "new ark client")

	arkServer := envOrDefault("E2E_ARK_SERVER_URL", defaultArkServer)
	explorerURL := envOrDefault("E2E_EXPLORER_URL", defaultExplorer)
	password := envOrDefault("E2E_ARK_PASSWORD", defaultPassword)

	initCtx, cancelInit := context.WithTimeout(context.Background(), 60*time.Second)
	err = arkClient.Init(initCtx, arksdk.InitArgs{
		ClientType:           client.GrpcClient,
		WalletType:           "singlekey",
		ServerUrl:            arkServer,
		Password:             password,
		ExplorerURL:          explorerURL,
		ExplorerPollInterval: 0,
		WithTransactionFeed:  false,
	})
	cancelInit()
	require.NoError(t, err, "init ark client")

	if err := arkClient.Unlock(ctx, password); err != nil {
		cancel()
		t.Fatalf("unlock ark client: %v", err)
	}

	privHex, err := arkClient.Dump(ctx)
	require.NoError(t, err, "dump ark private key")
	privBytes, err := hex.DecodeString(privHex)
	require.NoError(t, err, "decode private key")
	privateKey, _ := btcec.PrivKeyFromBytes(privBytes)
	publicKey := privateKey.PubKey()

	transportClient, err := grpcclient.NewClient(arkServer)
	require.NoError(t, err, "new transport client")

	indexerClient, err := indexergrpc.NewClient(arkServer)
	require.NoError(t, err, "new indexer client")

	boltzHTTP := envOrDefault("E2E_BOLTZ_HTTP", defaultBoltzHTTP)
	boltzWS := envOrDefault("E2E_BOLTZ_WS", defaultBoltzWS)
	boltzAPI := &boltz.Api{URL: boltzHTTP, WSURL: boltzWS}

	handler := swap.NewSwapHandler(
		arkClient,
		transportClient,
		indexerClient,
		boltzAPI,
		publicKey,
		defaultSwapTimout,
	)

	st := &SwapTest{
		T:          t,
		Ctx:        ctx,
		cancel:     cancel,
		Handler:    handler,
		ArkClient:  arkClient,
		Transport:  transportClient,
		Indexer:    indexerClient,
		Boltz:      boltzAPI,
		PublicKey:  publicKey,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		APIBaseURL: envOrDefault("E2E_FULMINE_HTTP_API", defaultHTTPAPI),
		Password:   password,
	}

	require.NoError(t, ensureArkFunds(ctx, arkClient), "fund ark client")

	t.Cleanup(st.Close)

	return st
}

// Close releases the resources allocated by the SwapTest harness.
func (st *SwapTest) Close() {
	if st.Transport != nil {
		st.Transport.Close()
	}
	if st.Indexer != nil {
		st.Indexer.Close()
	}
	if st.ArkClient != nil {
		st.ArkClient.Stop()
	}
	if st.cancel != nil {
		st.cancel()
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func ensureArkFunds(ctx context.Context, arkClient arksdk.ArkClient) error {
	balance, err := arkClient.Balance(ctx, false)
	if err != nil {
		return fmt.Errorf("ark balance: %w", err)
	}

	if balance.OnchainBalance.SpendableAmount >= desiredOnchainMin {
		return nil
	}

	onchainAddr, _, boardingAddr, err := arkClient.Receive(ctx)
	if err != nil {
		return fmt.Errorf("ark receive: %w", err)
	}

	for i := 0; i < arkFaucetRounds; i++ {
		if err := nigiriFaucet(ctx, strings.TrimSpace(onchainAddr), arkFaucetAmount); err != nil {
			return fmt.Errorf("faucet onchain: %w", err)
		}
		if boardingAddr != "" {
			if err := nigiriFaucet(ctx, strings.TrimSpace(boardingAddr), arkFaucetAmount); err != nil {
				return fmt.Errorf("faucet boarding: %w", err)
			}
		}
	}
	if _, err := runNigiri(ctx, "rpc", "--generate", "1"); err != nil {
		return fmt.Errorf("confirm faucet: %w", err)
	}

	time.Sleep(5 * time.Second)

	// settle funds
	if _, err := arkClient.Settle(ctx); err != nil {
		return fmt.Errorf("failed to settle: %w", err)
	}

	time.Sleep(5 * time.Second)

	return nil
}

func nigiriFaucet(ctx context.Context, address, amount string) error {
	if address == "" {
		return fmt.Errorf("empty faucet address")
	}
	_, err := runNigiri(ctx, "faucet", address, amount)
	return err
}

func runNigiri(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "nigiri", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("nigiri %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
