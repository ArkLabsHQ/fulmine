package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

const fulminePass = "password"

func TestChainSwapArkToBTC(t *testing.T) {
	ctx := context.Background()

	client, err := newFulmineClient(clientFulmineURL)
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
	require.NoError(t, err)

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	mineRegtestBlocks(t, ctx, 20)

	// Boltz rescans the Ark chain on an interval (rescanInterval=30 in the
	// regtest boltz config), so allow well over one rescan cycle for the claim.
	waitChainSwapStatus(t, ctx, client, swapID, "claimed", 90*time.Second)

	// Boltz's BTC payout needs a confirmation before scantxoutset (which scans
	// the confirmed UTXO set) sees it, so mine and re-scan until it lands.
	require.Eventually(t, func() bool {
		mineRegtestBlocks(t, ctx, 1)
		return nigiriScanAddressBalanceBTC(t, ctx, btcAddress) > 0
	}, 30*time.Second, 2*time.Second, "claimed BTC never confirmed at the destination address")
}

func TestChainSwapBTCtoARK(t *testing.T) {
	ctx := context.Background()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err)

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	// Fund the lockup with the exact amount Boltz quotes (swap amount + its
	// fee); a hardcoded under-payment leaves the swap stuck "pending".
	err = faucet(ctx, createResp.LockupAddress, float64(createResp.GetExpectedAmount())/1e8)
	require.NoError(t, err)

	// Boltz rescans the Ark chain on an interval (rescanInterval=30 in the
	// regtest boltz config), so allow well over one rescan cycle for the claim.
	waitChainSwapStatus(t, ctx, client, swapID, "claimed", 90*time.Second)

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

func TestChainSwapBTCtoARKWithQuote(t *testing.T) {
	ctx := context.Background()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err)

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	// Live Boltz rejects an over-funded lockup ("locked X is more than expected Y"
	// -> transaction.lockupFailed), so fund exactly what it quotes.
	err = faucet(ctx, createResp.LockupAddress, float64(createResp.GetExpectedAmount())/1e8)
	require.NoError(t, err)

	waitChainSwapStatus(t, ctx, client, swapID, "claimed", 90*time.Second)

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// cltvRequiredRE pulls the required block height out of a BTC→ARK unilateral
// refund "CLTV timeout not yet reached: ... required N" error.
var cltvRequiredRE = regexp.MustCompile(`required (\d+)`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

//// CHAIN SWAP REFUND TESTS AGAINST LIVE BOLTZ ////

func TestChainSwapArkToBTCCooperativeRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

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
	require.Empty(t, createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID)

	// ARK→BTC refund is cooperative (Boltz co-signs) and ends in "refunded".
	refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
	require.Equal(t, "refund initiated", refundResp.GetMessage())

	waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)

	balance, err = client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("vtxo balance after refund: %d", balance.GetAmount())
}

func TestChainSwapBTCToARKUnilateralRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err)
	require.Empty(t, createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID)
	lockupAddress := createResp.GetLockupAddress()
	require.NotEmpty(t, lockupAddress, "CreateChainSwap returned empty lockup address")
	expectedAmount := createResp.GetExpectedAmount()
	require.Greater(t, expectedAmount, uint64(0), "CreateChainSwap returned invalid expected amount")

	// Fund the lockup with exactly what Boltz quotes.
	fundAddressAndGetConfirmedTx(t, ctx, lockupAddress, expectedAmount)

	// BTC→ARK refund is unilateral; it can spend only once the BTC CLTV timeout
	// has passed, so mine past the lockup timeout height first.
	mineRegtestBlocksToHeight(t, ctx, int(createResp.GetTimeoutBlockHeight())+1)

	refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
	require.Equal(t, "refund initiated", refundResp.GetMessage())

	waitChainSwapStatus(t, ctx, client, swapID, "refunded_unilaterally", 60*time.Second)
}

func TestChainSwapRefundChainSwapRPC(t *testing.T) {
	t.Run("ark_to_btc_cooperative", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := nigiriGetNewAddress(t, ctx)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddress,
		})
		require.NoError(t, err)
		require.Empty(t, createResp.GetError())
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)

		// ARK→BTC refund is cooperative (Boltz co-signs) and ends in "refunded".
		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)
	})

	t.Run("btc_to_ark", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
			Amount:    3000,
		})
		require.NoError(t, err)
		require.Empty(t, createResp.GetError())
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)
		require.NotEmpty(t, createResp.GetLockupAddress())
		require.Greater(t, createResp.GetExpectedAmount(), uint64(0))

		// Fund the lockup with exactly what Boltz quotes.
		fundAddressAndGetConfirmedTx(
			t, ctx, createResp.GetLockupAddress(), createResp.GetExpectedAmount(),
		)

		// BTC→ARK refund is unilateral; mine past the BTC CLTV timeout first.
		mineRegtestBlocksToHeight(t, ctx, int(createResp.GetTimeoutBlockHeight())+1)

		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded_unilaterally", 60*time.Second)
	})
}

func TestChainSwapRecovery(t *testing.T) {
	t.Run("ark_to_btc_claim_real_boltz", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := nigiriGetNewAddress(t, ctx)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddress,
		})
		require.NoError(t, err)

		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)

		time.Sleep(3 * time.Second)
		// Restart the swap client (the user Fulmine) mid-swap to exercise recovery.
		restartDockerComposeServices(t, ctx, "fulmine-user")
		time.Sleep(3 * time.Second)
		err = unlockAndSettle(clientFulmineURL, fulminePass)
		require.NoError(t, err)

		mineRegtestBlocks(t, ctx, 20)
		waitChainSwapStatus(t, ctx, client, swapID, "claimed", 90*time.Second)

		addrBalance := nigiriScanAddressBalanceBTC(t, ctx, btcAddress)
		require.Greater(t, addrBalance, float64(0))
	})

	t.Run("ark_to_btc_refund", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := nigiriGetNewAddress(t, ctx)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddress,
		})
		require.NoError(t, err)
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)

		time.Sleep(3 * time.Second)
		// Restart the swap client (the user Fulmine) mid-swap to exercise recovery.
		restartDockerComposeServices(t, ctx, "fulmine-user")
		time.Sleep(3 * time.Second)
		err = unlockAndSettle(clientFulmineURL, fulminePass)
		require.NoError(t, err)

		// ARK→BTC refund is cooperative (Boltz co-signs) and ends in "refunded".
		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded", 40*time.Second)
	})

	t.Run("btc_to_ark_refund", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
			Amount:    3000,
		})
		require.NoError(t, err)
		swapID := createResp.GetId()
		require.NotEmpty(t, swapID)
		require.NotEmpty(t, createResp.GetLockupAddress())
		require.Greater(t, createResp.GetExpectedAmount(), uint64(0))

		// Fund the lockup with exactly what Boltz quotes.
		fundAddressAndGetConfirmedTx(
			t, ctx, createResp.GetLockupAddress(), createResp.GetExpectedAmount(),
		)

		time.Sleep(3 * time.Second)
		// Restart the swap client (the user Fulmine) mid-swap to exercise recovery.
		restartDockerComposeServices(t, ctx, "fulmine-user")
		time.Sleep(3 * time.Second)
		err = unlockAndSettle(clientFulmineURL, fulminePass)
		require.NoError(t, err)

		// BTC→ARK refund is unilateral; mine past the BTC CLTV timeout first.
		mineRegtestBlocksToHeight(t, ctx, int(createResp.GetTimeoutBlockHeight())+1)

		refundResp := refundChainSwapRPCWithRetry(t, ctx, client, swapID, 90*time.Second)
		require.Equal(t, "refund initiated", refundResp.GetMessage())

		waitChainSwapStatus(t, ctx, client, swapID, "refunded_unilaterally", 60*time.Second)
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
	swap := resp.GetSwaps()[0]
	if swap.GetErrorMessage() != "" {
		t.Logf("Swap %s error: %s", swapID, swap.GetErrorMessage())
	}
	require.Equal(t, expected, swap.GetStatus())
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
		// Bound each call so a flaky explorer/arkd makes the refund retry instead
		// of blocking on the (large) test context for minutes. The refund chains
		// several individually-bounded calls (explorer 10s each + arkd boarding
		// address), so under a starved stack the *cumulative* time, not any single
		// hang, is what overruns; fail fast and retry rather than widen the bound.
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		resp, err := client.RefundChainSwap(callCtx, &pb.RefundChainSwapRequest{Id: swapID})
		cancel()
		if err == nil {
			return resp
		}

		// Transient per-call deadline (flaky explorer/arkd); just retry.
		if strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "DeadlineExceeded") {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
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
		// BTC→ARK unilateral refund can only spend once the BTC CLTV timeout is
		// reached. CreateChainSwap under-reports that height, but the error names
		// the required one ("required 362"), so mine to it and retry.
		if strings.Contains(err.Error(), "CLTV timeout not yet reached") {
			if m := cltvRequiredRE.FindStringSubmatch(err.Error()); m != nil {
				var required int
				if _, scanErr := fmt.Sscanf(m[1], "%d", &required); scanErr == nil {
					mineRegtestBlocksToHeight(t, ctx, required+1)
				}
			}
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		require.NoError(t, err)
	}

	require.NoError(t, lastErr)
	return nil
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
	out, err := regtestCmd(ctx, "rpc", "getnewaddress")
	require.NoError(t, err)
	address := strings.TrimSpace(out)
	require.NotEmpty(t, address)
	return address
}

func nigiriScanAddressBalanceBTC(t *testing.T, ctx context.Context, addr string) float64 {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "scantxoutset", "start", fmt.Sprintf(`["addr(%s)"]`, addr))
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
	out, err := regtestCmd(ctx, "rpc", "sendtoaddress", address, amountBtc)
	require.NoError(t, err)
	txid := strings.TrimSpace(out)
	require.NotEmpty(t, txid)
	return txid
}

func nigiriGetRawTransaction(t *testing.T, ctx context.Context, txid string) string {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getrawtransaction", txid)
	require.NoError(t, err)
	txhex := strings.TrimSpace(out)
	require.NotEmpty(t, txhex)
	return txhex
}

func nigiriGetBlockchainInfo(t *testing.T, ctx context.Context) nigiriBlockchainInfo {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getblockchaininfo")
	require.NoError(t, err)

	var info nigiriBlockchainInfo
	require.NoError(t, json.Unmarshal([]byte(stripANSI(out)), &info))
	return info
}

func nigiriGetBlockCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getblockcount")
	require.NoError(t, err)

	var height int
	_, err = fmt.Sscanf(strings.TrimSpace(stripANSI(out)), "%d", &height)
	require.NoError(t, err)
	return height
}

func nigiriGenerateBlocks(t *testing.T, ctx context.Context, count int) {
	t.Helper()
	_, err := regtestCmd(ctx, "mine", fmt.Sprint(count))
	require.NoError(t, err)
}
