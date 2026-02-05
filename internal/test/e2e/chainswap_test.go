package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

func TestChainSwapArkToBTC(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	btcAddress := nigiriGetNewAddress(t, ctx)
	addrBalance := nigiriScanAddressBalanceBTC(t, ctx, btcAddress)
	require.Equal(t, addrBalance, float64(0))

	// Step 1: Create Ark→BTC chain swap
	t.Log("Creating Ark→BTC chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	mineRegtestBlocks(t, ctx, 20)

	time.Sleep(5 * time.Second)

	addrBalance = nigiriScanAddressBalanceBTC(t, ctx, btcAddress)
	require.Greater(t, addrBalance, float64(0))

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())
}

func TestChainSwapBTCtoARK(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	err = faucet(ctx, createResp.LockupAddress, 0.00003000)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

func TestChainSwapBTCtoARKWithQuote(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	// faucet bigger amount so that we can test quote
	err = faucet(ctx, createResp.LockupAddress, 0.00015500)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

//// CHAIN SWAP TESTS WITH MOCKED BOLTZ ////

const (
	mockFulmineGRPC = "localhost:7100"
	mockBoltzAdmin  = "http://localhost:9101"
)

type mockSwapState struct {
	ID               string `json:"id"`
	LastStatus       string `json:"lastStatus"`
	ServerLockAmount uint64 `json:"serverLockAmount"`
	BTCLockupAddress string `json:"btcLockupAddress"`
	QuoteGets        int    `json:"quoteGets"`
	QuoteAccepts     int    `json:"quoteAccepts"`
	ClaimRequests    int    `json:"claimRequests"`
	RefundRequests   int    `json:"refundRequests"`
}

func TestChainSwapMockArkToBTCScriptPathClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockReset(t)
	mockSetConfig(t, map[string]any{"claimMode": "fail", "refundMode": "success"})

	client, err := newFulmineClient(mockFulmineGRPC)
	require.NoError(t, err)
	require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

	btcAddress := nigiriGetNewAddress(t, ctx)
	btcBalanceBeforeSat := nigiriScanAddressBalanceSats(t, ctx, btcAddress)
	require.Equal(t, btcBalanceBeforeSat, 0)
	t.Logf(" BTC balance before swap: %d sat", btcBalanceBeforeSat)

	balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance before chain swap: %d", balance.GetAmount())

	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err)
	require.Emptyf(t, createResp.GetError(), "CreateChainSwap returned application error: %s", createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID, "CreateChainSwap returned empty swap id")
	require.NotEmpty(t, swapID)

	// For script-path claim we must provide a real, spendable server lockup tx from regtest.
	mockState := mockGetSwap(t, swapID)
	require.NotEmpty(t, mockState.BTCLockupAddress)
	require.Greater(t, mockState.ServerLockAmount, uint64(0))

	serverLockTxID, serverLockTxHex := fundAddressAndGetConfirmedTx(
		t,
		ctx,
		mockState.BTCLockupAddress,
		mockState.ServerLockAmount,
	)

	time.Sleep(1 * time.Second)
	mockPushEvent(t, swapID, "transaction.confirmed")
	mockPushEventWithTx(t, swapID, "transaction.server.confirmed", serverLockTxID, serverLockTxHex)

	waitChainSwapStatus(t, ctx, client, swapID, "claimed", 40*time.Second)

	state := mockGetSwap(t, swapID)
	require.Greater(t, state.ClaimRequests, 0, "expected initial cooperative claim attempt")

	balanceAfter, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance after chain swap: %d", balanceAfter.GetAmount())
	balanceDif := int64(balanceAfter.GetAmount()) - int64(balance.GetAmount())
	t.Logf("vtxo balance diff after chain swap: %d", balanceDif)

	btcBalanceAfterSat := nigiriScanAddressBalanceSats(t, ctx, btcAddress)
	t.Logf("BTC balance after swap: %d sat", btcBalanceAfterSat)
	t.Logf("BTC balance diff: %d sat", btcBalanceAfterSat-btcBalanceBeforeSat)
}

func TestChainSwapMockArkToBTCCooperativeRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockReset(t)
	mockSetConfig(t, map[string]any{"refundMode": "success"})

	client, err := newFulmineClient(mockFulmineGRPC)
	require.NoError(t, err)
	require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

	btcAddress := nigiriGetNewAddress(t, ctx)

	balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance before chain swap: %d", balance.GetAmount())

	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err)
	swapID := createResp.GetId()

	time.Sleep(2 * time.Second)

	balance, err = client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance after chain swap: %d", balance.GetAmount())

	time.Sleep(1 * time.Second)
	mockPushEvent(t, swapID, "swap.expired")

	waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)

	state := mockGetSwap(t, swapID)
	require.Greater(t, state.RefundRequests, 0, "expected cooperative refund call to mock boltz")

	balance, err = client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance after refund: %d", balance.GetAmount())
}

func TestChainSwapMockArkToBTCUnilateralRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	mockReset(t)
	mockSetConfig(t, map[string]any{
		"refundMode": "fail",
	})

	client, err := newFulmineClient(mockFulmineGRPC)
	require.NoError(t, err)
	require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

	chainMedianTime := regtestMedianTime(t, ctx)
	refundAtUnix := chainMedianTime - 60
	mockSetConfig(t, map[string]any{
		"arkRefundAtUnix": refundAtUnix,
	})
	t.Logf("configured mock arkRefundAtUnix=%d (regtest mediantime=%d)", refundAtUnix, chainMedianTime)

	btcAddress := nigiriGetNewAddress(t, ctx)

	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err)
	require.Emptyf(t, createResp.GetError(), "CreateChainSwap returned application error: %s", createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID, "CreateChainSwap returned empty swap id")

	time.Sleep(1 * time.Second)
	mockPushEvent(t, swapID, "swap.expired")

	waitChainSwapStatus(t, ctx, client, swapID, "refunded", 80*time.Second)

	state := mockGetSwap(t, swapID)
	require.Greater(t, state.RefundRequests, 0, "expected failed cooperative refund attempt before unilateral refund")
}

func TestChainSwapMockBTCToARKUnilateralRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockReset(t)

	currentHeight := regtestBlockHeight(t, ctx)
	timeoutHeight := uint32(currentHeight + 2)
	if timeoutHeight < 144 {
		timeoutHeight = 144
	}
	mockSetConfig(t, map[string]any{
		"btcLockupTimeoutBlocks": timeoutHeight,
	})

	client, err := newFulmineClient(mockFulmineGRPC)
	require.NoError(t, err)
	require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err)
	lockupAddress := createResp.GetLockupAddress()
	require.NotEmpty(t, lockupAddress, "CreateChainSwap returned empty lockup address")
	expectedAmount := createResp.GetExpectedAmount()
	require.Greater(t, expectedAmount, uint64(0), "CreateChainSwap returned invalid expected amount")

	userLockTxID, userLockTxHex := fundAddressAndGetConfirmedTx(t, ctx, lockupAddress, expectedAmount)

	time.Sleep(3 * time.Second)
	mockPushEventWithTx(t, createResp.GetId(), "transaction.confirmed", userLockTxID, userLockTxHex)
	waitForEsploraTxIndexed(t, userLockTxID, 15*time.Second)

	// Unilateral BTC refund path can spend only after locktime is reached.
	mineRegtestBlocksToHeight(t, ctx, int(createResp.GetTimeoutBlockHeight())+1)

	// Simulate Boltz-side failure after user lockup to trigger refund logic.
	mockPushEvent(t, createResp.GetId(), "transaction.failed")

	waitChainSwapStatus(t, ctx, client, createResp.GetId(), "refunded", 60*time.Second)
}

func TestChainSwapMockRefundChainSwapRPC(t *testing.T) {
	t.Run("ark_to_btc_cooperative", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		mockReset(t)
		mockSetConfig(t, map[string]any{"refundMode": "success"})

		client, err := newFulmineClient(mockFulmineGRPC)
		require.NoError(t, err)
		require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

		btcAddress := nigiriGetNewAddress(t, ctx)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddress,
		})
		require.NoError(t, err)
		require.Emptyf(t, createResp.GetError(), "CreateChainSwap returned application error: %s", createResp.GetError())
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)

		time.Sleep(3 * time.Second)

		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 20*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)

		state := mockGetSwap(t, swapID)
		require.Greater(t, state.RefundRequests, 0, "expected cooperative refund call to mock boltz")
	})

	t.Run("btc_to_ark", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		mockReset(t)

		currentHeight := regtestBlockHeight(t, ctx)
		timeoutHeight := uint32(currentHeight + 2)
		if timeoutHeight < 144 {
			timeoutHeight = 144
		}
		mockSetConfig(t, map[string]any{
			"btcLockupTimeoutBlocks": timeoutHeight,
		})

		client, err := newFulmineClient(mockFulmineGRPC)
		require.NoError(t, err)
		require.NoError(t, refillFulmine(ctx, mockFulmineGRPC))

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
			Amount:    3000,
		})
		require.NoError(t, err)
		require.Emptyf(t, createResp.GetError(), "CreateChainSwap returned application error: %s", createResp.GetError())
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)
		require.NotEmpty(t, createResp.GetLockupAddress())
		require.Greater(t, createResp.GetExpectedAmount(), uint64(0))

		userLockTxID, userLockTxHex := fundAddressAndGetConfirmedTx(
			t, ctx, createResp.GetLockupAddress(), createResp.GetExpectedAmount(),
		)
		mockPushEventWithTx(t, swapID, "transaction.confirmed", userLockTxID, userLockTxHex)
		waitChainSwapStatus(t, ctx, client, swapID, "user_locked", 20*time.Second)
		waitForEsploraTxIndexed(t, userLockTxID, 15*time.Second)

		mineRegtestBlocksToHeight(t, ctx, int(createResp.GetTimeoutBlockHeight())+1)

		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 15*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)
	})
}

func waitChainSwapStatus(
	t *testing.T,
	ctx context.Context,
	client pb.ServiceClient,
	swapID, expected string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{SwapIds: []string{swapID}})
		if err == nil && len(resp.GetSwaps()) > 0 {
			if resp.GetSwaps()[0].GetStatus() == expected {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	resp, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{SwapIds: []string{swapID}})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetSwaps())
	require.Equalf(t, expected, resp.GetSwaps()[0].GetStatus(), "final status mismatch for swap %s", swapID)
}

func waitForQuoteAcceptance(t *testing.T, swapID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := mockGetSwap(t, swapID)
		if state.QuoteAccepts > 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	state := mockGetSwap(t, swapID)
	require.Greater(t, state.QuoteAccepts, 0, "expected quote acceptance for swap %s", swapID)
}

func refundChainSwapRPCWithRetry(
	t *testing.T,
	ctx context.Context,
	client pb.ServiceClient,
	swapID string,
	timeout time.Duration,
) *pb.RefundChainSwapResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		resp, err := client.RefundChainSwap(ctx, &pb.RefundChainSwapRequest{Id: swapID})
		if err == nil {
			return resp
		}

		// Esplora can lag a bit in regtest; retry while lockup tx is not yet indexed.
		if strings.Contains(err.Error(), "failed to fetch lockup transaction from explorer") &&
			strings.Contains(err.Error(), "status 404") {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		// ARK indexer can lag after SendOffChain; retry until VHTLC appears.
		if strings.Contains(err.Error(), "no vtxos found for vhtlc") {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		require.NoError(t, err)
	}

	require.NoError(t, lastErr)
	return nil
}

func waitForEsploraTxIndexed(t *testing.T, txID string, timeout time.Duration) {
	t.Helper()
	require.NotEmpty(t, txID)

	baseURL := os.Getenv("MOCK_ESPLORA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/tx/" + txID + "/hex"

	deadline := time.Now().Add(timeout)
	lastStatus := 0
	lastBody := ""

	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint) //nolint:noctx
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			_ = resp.Body.Close()
			lastStatus = resp.StatusCode
			lastBody = strings.TrimSpace(string(body))
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	require.Failf(
		t,
		"esplora tx index timeout",
		"tx %s not indexed at %s within %v (last status=%d body=%s). Set MOCK_ESPLORA_URL if needed.",
		txID, endpoint, timeout, lastStatus, lastBody,
	)
}

func mockReset(t *testing.T) {
	t.Helper()
	mockPost(t, "/admin/reset", nil, nil)
}

func mockSetConfig(t *testing.T, cfg map[string]any) {
	t.Helper()
	mockPost(t, "/admin/config", cfg, nil)
}

func mockPushEvent(t *testing.T, swapID, status string) {
	t.Helper()
	mockPost(t, fmt.Sprintf("/admin/swaps/%s/event", swapID), map[string]any{"status": status}, nil)
}

func mockPushEventWithTx(t *testing.T, swapID, status, txid, txhex string) {
	t.Helper()
	mockPost(t, fmt.Sprintf("/admin/swaps/%s/event", swapID), map[string]any{
		"status": status,
		"txid":   txid,
		"txhex":  txhex,
	}, nil)
}

func fundAddressAndGetConfirmedTx(t *testing.T, ctx context.Context, address string, sats uint64) (string, string) {
	t.Helper()
	amountBtc := fmt.Sprintf("%d.%08d", sats/100000000, sats%100000000)

	txid := nigiriSendToAddress(t, ctx, address, amountBtc)
	mineRegtestBlocks(t, ctx, 10)
	txhex := nigiriGetRawTransaction(t, ctx, txid)

	return txid, txhex
}

func regtestMedianTime(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	info := nigiriGetBlockchainInfo(t, ctx)

	if info.MedianTime > 0 {
		return info.MedianTime
	}
	require.Greater(t, info.Time, int64(0), "missing regtest chain time from getblockchaininfo")
	return info.Time
}

func regtestBlockHeight(t *testing.T, ctx context.Context) int {
	t.Helper()
	return nigiriGetBlockCount(t, ctx)
}

func mineRegtestBlocks(t *testing.T, ctx context.Context, count int) {
	t.Helper()
	if count <= 0 {
		return
	}
	nigiriGenerateBlocks(t, ctx, count)
}

func mineRegtestBlocksToHeight(t *testing.T, ctx context.Context, target int) {
	t.Helper()
	current := regtestBlockHeight(t, ctx)
	if current >= target {
		return
	}
	mineRegtestBlocks(t, ctx, target-current)
}

type nigiriBlockchainInfo struct {
	MedianTime int64 `json:"mediantime"`
	Time       int64 `json:"time"`
}

func nigiriGetNewAddress(t *testing.T, ctx context.Context) string {
	t.Helper()
	out, err := runCommand(ctx, "nigiri rpc getnewaddress")
	require.NoError(t, err)
	address := strings.TrimSpace(out)
	require.NotEmpty(t, address)
	return address
}

func nigiriScanAddressBalanceBTC(t *testing.T, ctx context.Context, addr string) float64 {
	t.Helper()
	out, err := runCommand(ctx, fmt.Sprintf("nigiri rpc scantxoutset start '[\"addr(%s)\"]'", addr))
	require.NoError(t, err)

	var raw struct {
		TotalAmount float64 `json:"total_amount"`
	}
	require.NoError(t, json.Unmarshal([]byte(stripANSI(out)), &raw))
	return raw.TotalAmount
}

func nigiriScanAddressBalanceSats(t *testing.T, ctx context.Context, addr string) int {
	t.Helper()
	return int(nigiriScanAddressBalanceBTC(t, ctx, addr) * 100_000_000)
}

func nigiriSendToAddress(t *testing.T, ctx context.Context, address, amountBtc string) string {
	t.Helper()
	out, err := runCommand(ctx, fmt.Sprintf("nigiri rpc sendtoaddress %s %s", address, amountBtc))
	require.NoError(t, err)
	txid := strings.TrimSpace(out)
	require.NotEmpty(t, txid)
	return txid
}

func nigiriGetRawTransaction(t *testing.T, ctx context.Context, txid string) string {
	t.Helper()
	out, err := runCommand(ctx, fmt.Sprintf("nigiri rpc getrawtransaction %s", txid))
	require.NoError(t, err)
	txhex := strings.TrimSpace(out)
	require.NotEmpty(t, txhex)
	return txhex
}

func nigiriGetBlockchainInfo(t *testing.T, ctx context.Context) nigiriBlockchainInfo {
	t.Helper()
	out, err := runCommand(ctx, "nigiri rpc getblockchaininfo")
	require.NoError(t, err)

	var info nigiriBlockchainInfo
	require.NoError(t, json.Unmarshal([]byte(stripANSI(out)), &info))
	return info
}

func nigiriGetBlockCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	out, err := runCommand(ctx, "nigiri rpc getblockcount")
	require.NoError(t, err)

	var height int
	_, err = fmt.Sscanf(strings.TrimSpace(stripANSI(out)), "%d", &height)
	require.NoError(t, err)
	return height
}

func nigiriGenerateBlocks(t *testing.T, ctx context.Context, count int) {
	t.Helper()
	address := nigiriGetNewAddress(t, ctx)
	_, err := runCommand(ctx, fmt.Sprintf("nigiri rpc generatetoaddress %d %s", count, address))
	require.NoError(t, err)
}

func mockGetSwap(t *testing.T, swapID string) mockSwapState {
	t.Helper()
	var state mockSwapState
	mockGet(t, fmt.Sprintf("/admin/swaps/%s", swapID), &state)
	return state
}

func mockGet(t *testing.T, path string, out any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, mockBoltzAdmin+path, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equalf(t, http.StatusOK, resp.StatusCode, "GET %s failed", path)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

func mockPost(t *testing.T, path string, body any, out any) {
	t.Helper()
	payload := []byte("{}")
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req, err := http.NewRequest(http.MethodPost, mockBoltzAdmin+path, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var serverErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&serverErr)
		require.Failf(t, "mock post failed", "POST %s status=%d body=%v", path, resp.StatusCode, serverErr)
	}

	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
}
