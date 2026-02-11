package e2e_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/arkade-os/go-sdk/wallet"
	singlekeywallet "github.com/arkade-os/go-sdk/wallet/singlekey"
	inmemorystore "github.com/arkade-os/go-sdk/wallet/singlekey/store/inmemory"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	lnd = "docker exec lnd lncli --network=regtest"
	cln = "docker exec cln lightning-cli --network=regtest"
)

func newFulmineClient(url string) (pb.ServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewServiceClient(conn), nil
}

func lndAddInvoice(ctx context.Context, sats int) (string, string, error) {
	command := fmt.Sprintf("%s addinvoice --amt %d", lnd, sats)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentRequest string `json:"payment_request"`
		RHash          string `json:"r_hash"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return "", "", err
	}
	return resp.PaymentRequest, resp.RHash, nil
}

func lndPayInvoice(ctx context.Context, invoice string) error {
	command := fmt.Sprintf("%s payinvoice --force %s", lnd, invoice)
	_, err := runCommand(ctx, command)
	return err
}

func lndCancelInvoice(ctx context.Context, rHash string) error {
	command := fmt.Sprintf("%s cancelinvoice %s", lnd, rHash)
	_, err := runCommand(ctx, command)
	return err
}

func clnAddOffer(ctx context.Context, sats int) (string, string, error) {
	label := fmt.Sprintf("funding-%s", time.Now().Format(time.RFC3339))
	command := fmt.Sprintf(`%s offer %d "%s"`, cln, sats, label)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentHash string `json:"offer_id"`
		Bolt11      string `json:"bolt12"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		return "", "", err
	}
	return resp.Bolt11, resp.PaymentHash, nil
}

func faucet(ctx context.Context, address string, amount float64) error {
	command := fmt.Sprintf("nigiri faucet %s %.8f", address, amount)
	_, err := runCommand(ctx, command)
	return err
}

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = ptmx.Close() }()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		done <- err
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", ctx.Err()
	case copyErr := <-done:
		if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
			return "", fmt.Errorf("read command output: %w", copyErr)
		}
		if err := cmd.Wait(); err != nil {
			return "", fmt.Errorf("%s", strings.TrimSpace(out.String()))
		}
		return out.String(), nil
	}
}

func generateNote(t *testing.T, amount uint64) string {
	adminHttpClient := &http.Client{
		Timeout: 15 * time.Second,
	}

	reqBody := bytes.NewReader([]byte(fmt.Sprintf(`{"amount": "%d"}`, amount)))
	req, err := http.NewRequest("POST", "http://localhost:7071/v1/admin/note", reqBody)
	if err != nil {
		t.Fatalf("failed to prepare note request: %s", err)
	}
	req.Header.Set("Authorization", "Basic YWRtaW46YWRtaW4=")
	req.Header.Set("Content-Type", "application/json")

	resp, err := adminHttpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create note: %s", err)
	}
	defer resp.Body.Close()

	var noteResp struct {
		Notes []string `json:"notes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&noteResp); err != nil {
		t.Fatalf("failed to parse response: %s", err)
	}
	if len(noteResp.Notes) == 0 {
		t.Fatalf("no notes returned from admin API")
	}

	return noteResp.Notes[0]
}

func faucetOffchain(t *testing.T, client arksdk.ArkClient, amount float64) types.Vtxo {
	_, offchainAddr, _, err := client.Receive(t.Context())
	require.NoError(t, err)

	note := generateNote(t, uint64(amount*1e8))

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []types.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = client.NotifyIncomingFunds(t.Context(), offchainAddr)
		wg.Done()
	}()

	txid, err := client.RedeemNotes(t.Context(), []string{note})
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	wg.Wait()

	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	time.Sleep(time.Second)
	return incomingFunds[0]
}

func newDelegatorClient(url string) (pb.DelegatorServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewDelegatorServiceClient(conn), nil
}

func setupArkSDKwithPublicKey(
	t *testing.T,
) (arksdk.ArkClient, wallet.WalletService, *btcec.PublicKey, client.TransportClient) {
	serverUrl := "localhost:7070"
	password := "pass"

	appDataStore, err := store.NewStore(store.Config{
		ConfigStoreType:  types.InMemoryStore,
		AppDataStoreType: types.KVStore,
	})
	require.NoError(t, err)

	client, err := arksdk.NewArkClient(appDataStore)
	require.NoError(t, err)

	walletStore, err := inmemorystore.NewWalletStore()
	require.NoError(t, err)
	require.NotNil(t, walletStore)

	wallet, err := singlekeywallet.NewBitcoinWallet(appDataStore.ConfigStore(), walletStore)
	require.NoError(t, err)

	privkey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	privkeyHex := hex.EncodeToString(privkey.Serialize())

	err = client.InitWithWallet(context.Background(), arksdk.InitWithWalletArgs{
		Wallet:     wallet,
		ClientType: arksdk.GrpcClient,
		ServerUrl:  serverUrl,
		Password:   password,
		Seed:       privkeyHex,
	})
	require.NoError(t, err)

	err = client.Unlock(context.Background(), password)
	require.NoError(t, err)

	grpcClient, err := grpcclient.NewClient(serverUrl)
	require.NoError(t, err)

	return client, wallet, privkey.PubKey(), grpcClient
}
