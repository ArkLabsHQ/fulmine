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

	btcAddress := btcGetNewAddress(t, ctx)
	addrBalance := btcScanAddressBalanceBTC(t, ctx, btcAddress)
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
		return btcScanAddressBalanceBTC(t, ctx, btcAddress) > 0
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

	btcAddress := btcGetNewAddress(t, ctx)

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

// TestChainSwapBTCToARKUnilateralRefund exercises the unilateral BTC refund:
// the user locks BTC but Boltz never claims it, so after the lockup's CLTV
// timeout the user reclaims the BTC on-chain by spending the refund leaf.
//
// With a cooperating Boltz the swap always completes (fulmine claims the ARK
// VHTLC, Boltz then claims the BTC lockup), so the unilateral path is
// unreachable. We force "Boltz never claims" by stopping the Boltz container
// after creation but before it locks the ARK side — the same lever the
// dotnet-sdk e2e suite uses (NArk.Tests.End2End/Common/DockerHelper).
func TestChainSwapBTCToARKUnilateralRefund(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	// Boltz must be up to quote + create the swap.
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err)
	require.Empty(t, createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID)

	// Restart Boltz at the end so later tests get a healthy Boltz.
	defer startBoltzAndWait(t)

	// Fund the lockup but DON'T confirm it yet (faucet() always mines, so call
	// the regtest faucet directly without --confirm). Boltz reports the mempool
	// lockup so fulmine records the txid (user_locked), but Boltz won't lock the
	// ARK side until the lockup confirms — a race-free window to stop Boltz.
	_, err = regtestCmd(ctx, "faucet", createResp.LockupAddress,
		fmt.Sprintf("%.8f", float64(createResp.GetExpectedAmount())/1e8))
	require.NoError(t, err)

	// Stop Boltz the instant the swap is user_locked (fulmine has recorded the
	// lockup txid, but Boltz hasn't locked ARK yet). Log the progression so we
	// can see the window.
	start := time.Now()
	var last string
	stopped := false
	winDeadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(winDeadline) && !stopped {
		resp, e := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{SwapIds: []string{swapID}})
		if e == nil && len(resp.GetSwaps()) > 0 {
			s := resp.GetSwaps()[0].GetStatus()
			if s != last {
				t.Logf("STATUS t+%.1fs: %s", time.Since(start).Seconds(), s)
				last = s
			}
			switch s {
			case "user_locked":
				stopBoltz(t, ctx)            // freeze Boltz before it can lock ARK
				mineRegtestBlocks(t, ctx, 1) // confirm the lockup with Boltz down
				stopped = true
			case "server_locked", "claimed":
				t.Fatalf("missed window: swap already %s at t+%.1fs", s, time.Since(start).Seconds())
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.True(t, stopped, "never observed user_locked within 90s (last=%s)", last)

	// Surface the lockup's CLTV-required height (the BTC refund leaf's timeout).
	var required int
	for i := 0; i < 30; i++ {
		cctx, cancelC := context.WithTimeout(ctx, 20*time.Second)
		_, e := client.RefundChainSwap(cctx, &pb.RefundChainSwapRequest{Id: swapID})
		cancelC()
		require.Error(t, e, "refund before the CLTV timeout should error")
		if m := cltvRequiredRE.FindStringSubmatch(stripANSI(e.Error())); m != nil {
			_, _ = fmt.Sscanf(m[1], "%d", &required)
			break
		}
		time.Sleep(time.Second)
	}
	require.NotZero(t, required, "never surfaced the CLTV required height")

	// Burst-mine past the CLTV in one call — the esplora tracks the node with no
	// lag, so this clears the refund's CLTV gate. (A per-block background miner
	// is ~10s/block on this host and can't outrun an ~80-block timeout.)
	mineRegtestBlocksToHeight(t, ctx, required+2)
	t.Logf("CLTV required %d; mined node to %d", required, regtestBlockHeight(t, ctx))

	// Feed a few confirmation blocks while the synchronous refund broadcasts the
	// on-chain refund tx and boards the coin via an arkd round.
	mineCtx, stopMiner := context.WithCancel(ctx)
	defer stopMiner()
	go func() {
		for {
			select {
			case <-mineCtx.Done():
				return
			case <-time.After(3 * time.Second):
				_, _ = regtestCmd(mineCtx, "mine", "1")
			}
		}
	}()

	// Retry the refund: after the burst the esplora needs a few seconds to index
	// up to the CLTV height; once it does, the call broadcasts and (with the
	// background miner) confirms + settles. A long per-call budget keeps the
	// synchronous refund from being cancelled mid-flight.
	refDeadline := time.Now().Add(4 * time.Minute)
	for {
		cctx, cancelC := context.WithTimeout(ctx, 3*time.Minute)
		_, e := client.RefundChainSwap(cctx, &pb.RefundChainSwapRequest{Id: swapID})
		cancelC()
		if e == nil {
			break
		}
		msg := stripANSI(e.Error())
		retryable := strings.Contains(msg, "CLTV timeout not yet reached") ||
			strings.Contains(msg, "failed to fetch lockup transaction") ||
			strings.Contains(msg, "no vtxos found for vhtlc")
		require.Truef(t, retryable && time.Now().Before(refDeadline),
			"unilateral refund failed: %s", msg)
		time.Sleep(2 * time.Second)
	}

	waitChainSwapStatus(t, ctx, client, swapID, "refunded_unilaterally", 90*time.Second)
}

// stopBoltz / startBoltzAndWait control the shared Boltz container to force
// non-cooperative scenarios (mirrors the dotnet-sdk DockerHelper approach).
// startBoltzAndWait uses a fresh context so the restore runs even if the test's
// own context was cancelled.
func stopBoltz(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := runCommand(ctx, "docker stop boltz"); err != nil {
		t.Fatalf("stop boltz: %v", err)
	}
}

func startBoltzAndWait(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := runCommand(ctx, "docker start boltz"); err != nil {
		t.Logf("start boltz: %v", err)
		return
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := runCommand(ctx, "docker inspect -f '{{.State.Status}}' boltz")
		if err == nil && strings.TrimSpace(out) == "running" {
			time.Sleep(5 * time.Second) // let Boltz's REST settle
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("boltz not running within 2m after restart")
}

// TestChainSwapArkToBTCUnilateralRefund exercises the unilateral ARK refund: the
// user locks an ARK VHTLC but Boltz never claims it, so after the VHTLC's
// "without receiver" relative locktime (224 blocks — Boltz is gone) the user
// reclaims the ARK. fulmine auto-locks the VHTLC (an arkd op, independent of
// Boltz), so we just stop Boltz before its ~30s rescan locks the BTC side.
func TestChainSwapArkToBTCUnilateralRefund(t *testing.T) {
	t.Skip("ARK->BTC unilateral refund is not reliably e2e-testable: the swap " +
		"auto-completes in <1s with no unconfirmed-lockup hold point (unlike " +
		"BTC->ARK). docker pause CAN freeze Boltz at user_locked, but the race is " +
		"flaky — it often loses to Boltz locking the BTC side first. Cover this " +
		"fund-safety path with a pkg/swap unit test instead; the BTC->ARK " +
		"direction (TestChainSwapBTCToARKUnilateralRefund) is the reliably " +
		"e2e-testable one. Code below is kept as a reference for the pause-" +
		"intercept and the ARK 224-block settlement-timing mechanics.")

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	client, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)
	defer startBoltzAndWait(t)

	btcAddress := btcGetNewAddress(t, ctx)
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err)
	require.Empty(t, createResp.GetError())
	swapID := createResp.GetId()
	require.NotEmpty(t, swapID)

	bal, _ := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	t.Logf("ARK balance before swap: %d; createResp=%+v", bal.GetAmount(), createResp)

	// Pause Boltz immediately — docker pause is instant (unlike stop's graceful
	// shutdown), racing to freeze it before it sees the ARK VHTLC and completes
	// the swap (which it does in <1s here).
	_, _ = runCommand(ctx, "docker pause boltz")
	defer func() { _, _ = runCommand(context.Background(), "docker unpause boltz") }()

	// Wait for fulmine to lock the ARK VHTLC (user_locked).
	start := time.Now()
	var last string
	locked := false
	winDeadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(winDeadline) && !locked {
		resp, e := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{SwapIds: []string{swapID}})
		if e == nil && len(resp.GetSwaps()) > 0 {
			sw := resp.GetSwaps()[0]
			s := sw.GetStatus()
			if s != last {
				t.Logf("STATUS t+%.1fs: %s  swap=%+v", time.Since(start).Seconds(), s, sw)
				last = s
			}
			switch s {
			case "user_locked":
				locked = true
			case "claimed":
				t.Fatalf("swap completed despite Boltz stopped (status=%s) at t+%.1fs", s, time.Since(start).Seconds())
			}
		}
		time.Sleep(2 * time.Second)
	}
	require.True(t, locked, "fulmine never locked the ARK VHTLC with Boltz down (last=%s)", last)

	// The without-receiver locktime (224 blocks) is RELATIVE to the VTXO's
	// confirmation, and the ARK VHTLC only confirms after a (time-based) arkd
	// round commits it on-chain. Wait for the round, then burst-mine to confirm
	// the commitment AND clear the 224-block locktime from there.
	time.Sleep(60 * time.Second)
	mineRegtestBlocks(t, ctx, 320)

	// Drive the refund. Background miner feeds confirmation blocks + a long
	// per-call budget as in the BTC->ARK case.
	mineCtx, stopMiner := context.WithCancel(ctx)
	defer stopMiner()
	go func() {
		for {
			select {
			case <-mineCtx.Done():
				return
			case <-time.After(3 * time.Second):
				_, _ = regtestCmd(mineCtx, "mine", "1")
			}
		}
	}()

	refDeadline := time.Now().Add(4 * time.Minute)
	for {
		cctx, cancelC := context.WithTimeout(ctx, 3*time.Minute)
		_, e := client.RefundChainSwap(cctx, &pb.RefundChainSwapRequest{Id: swapID})
		cancelC()
		if e == nil {
			break
		}
		msg := stripANSI(e.Error())
		t.Logf("refund attempt error: %s", msg)
		require.Truef(t, time.Now().Before(refDeadline), "ARK->BTC unilateral refund failed: %s", msg)
		time.Sleep(3 * time.Second)
	}
	waitChainSwapStatus(t, ctx, client, swapID, "refunded_unilaterally", 4*time.Minute)
}

func TestChainSwapRefundChainSwapRPC(t *testing.T) {
	t.Run("ark_to_btc_cooperative", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := btcGetNewAddress(t, ctx)

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
}

func TestChainSwapRecovery(t *testing.T) {
	t.Run("ark_to_btc_claim_real_boltz", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := btcGetNewAddress(t, ctx)

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

		addrBalance := btcScanAddressBalanceBTC(t, ctx, btcAddress)
		require.Greater(t, addrBalance, float64(0))
	})

	t.Run("ark_to_btc_refund", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		client, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		btcAddress := btcGetNewAddress(t, ctx)

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

	txid := btcSendToAddress(t, ctx, address, amountBtc)
	mineRegtestBlocks(t, ctx, 10)
	txhex := btcGetRawTransaction(t, ctx, txid)

	return txid, txhex
}

func regtestMedianTime(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	info := btcGetBlockchainInfo(t, ctx)

	if info.MedianTime > 0 {
		return info.MedianTime
	}
	require.Greater(t, info.Time, int64(0), "missing regtest chain time from getblockchaininfo")
	return info.Time
}

func regtestBlockHeight(t *testing.T, ctx context.Context) int {
	t.Helper()
	return btcGetBlockCount(t, ctx)
}

func mineRegtestBlocks(t *testing.T, ctx context.Context, count int) {
	t.Helper()
	if count <= 0 {
		return
	}
	btcGenerateBlocks(t, ctx, count)
}

func mineRegtestBlocksToHeight(t *testing.T, ctx context.Context, target int) {
	t.Helper()
	current := regtestBlockHeight(t, ctx)
	if current >= target {
		return
	}
	mineRegtestBlocks(t, ctx, target-current)
}

type btcBlockchainInfo struct {
	MedianTime int64 `json:"mediantime"`
	Time       int64 `json:"time"`
}

func btcGetNewAddress(t *testing.T, ctx context.Context) string {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getnewaddress")
	require.NoError(t, err)
	address := strings.TrimSpace(out)
	require.NotEmpty(t, address)
	return address
}

func btcScanAddressBalanceBTC(t *testing.T, ctx context.Context, addr string) float64 {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "scantxoutset", "start", fmt.Sprintf(`["addr(%s)"]`, addr))
	require.NoError(t, err)

	var raw struct {
		TotalAmount float64 `json:"total_amount"`
	}
	require.NoError(t, json.Unmarshal([]byte(stripANSI(out)), &raw))
	return raw.TotalAmount
}

func btcScanAddressBalanceSats(t *testing.T, ctx context.Context, addr string) int {
	t.Helper()
	return int(btcScanAddressBalanceBTC(t, ctx, addr) * 100_000_000)
}

func btcSendToAddress(t *testing.T, ctx context.Context, address, amountBtc string) string {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "sendtoaddress", address, amountBtc)
	require.NoError(t, err)
	txid := strings.TrimSpace(out)
	require.NotEmpty(t, txid)
	return txid
}

func btcGetRawTransaction(t *testing.T, ctx context.Context, txid string) string {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getrawtransaction", txid)
	require.NoError(t, err)
	txhex := strings.TrimSpace(out)
	require.NotEmpty(t, txhex)
	return txhex
}

func btcGetBlockchainInfo(t *testing.T, ctx context.Context) btcBlockchainInfo {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getblockchaininfo")
	require.NoError(t, err)

	var info btcBlockchainInfo
	require.NoError(t, json.Unmarshal([]byte(stripANSI(out)), &info))
	return info
}

func btcGetBlockCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	out, err := regtestCmd(ctx, "rpc", "getblockcount")
	require.NoError(t, err)

	var height int
	_, err = fmt.Sscanf(strings.TrimSpace(stripANSI(out)), "%d", &height)
	require.NoError(t, err)
	return height
}

func btcGenerateBlocks(t *testing.T, ctx context.Context, count int) {
	t.Helper()
	_, err := regtestCmd(ctx, "mine", fmt.Sprint(count))
	require.NoError(t, err)
}
