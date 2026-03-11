package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

// TestEventStreamVHTLCLifecycle verifies that the GetEventStream gRPC
// server-side streaming endpoint delivers htlc_created and htlc_spent
// (CLAIMED) events during a VHTLC create -> fund -> claim lifecycle.
//
// Flow:
//  1. Open a gRPC event stream.
//  2. Create a VHTLC (expect htlc_created event).
//  3. Fund the VHTLC with an offchain send.
//  4. Claim the VHTLC with the preimage (expect htlc_spent CLAIMED event).
//  5. Verify events received on the stream in the correct order.
func TestEventStreamVHTLCLifecycle(t *testing.T) {
	f, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	ctx := t.Context()

	// Get wallet info (pubkey used as receiver for self-send VHTLC)
	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info.GetPubkey())

	// Open the event stream before performing any VHTLC operations so we
	// are guaranteed to capture all events.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := f.GetEventStream(streamCtx, &pb.GetEventStreamRequest{})
	require.NoError(t, err)

	// Collect events in background goroutine.
	var (
		mu     sync.Mutex
		events []*pb.GetEventStreamResponse
	)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return // stream closed or cancelled
			}
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}()

	// Generate a random preimage and derive the hash.
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	// Step 1: Create the VHTLC (sender = receiver = self for simplicity).
	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
		UnilateralClaimDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 1024,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, vhtlcResp.GetId())
	require.NotEmpty(t, vhtlcResp.GetAddress())

	vhtlcId := vhtlcResp.GetId()
	t.Logf("created VHTLC %s at address %s", vhtlcId, vhtlcResp.GetAddress())

	// The htlc_created event is emitted asynchronously in a goroutine
	// after DB persistence, so allow a short window for it to arrive.
	time.Sleep(2 * time.Second)

	// Step 2: Fund the VHTLC with an offchain send.
	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.GetAddress(),
		Amount:  1000,
	})
	require.NoError(t, err)
	t.Log("funded VHTLC via SendOffChain")

	// Step 3: Claim the VHTLC with the preimage.
	claimResp, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlcId,
		Preimage: hex.EncodeToString(preimage),
	})
	require.NoError(t, err)
	require.NotEmpty(t, claimResp.GetRedeemTxid())
	t.Logf("claimed VHTLC, redeem txid: %s", claimResp.GetRedeemTxid())

	// Allow time for all events to propagate through the stream.
	time.Sleep(3 * time.Second)

	// Cancel the stream context to cleanly close the connection.
	streamCancel()
	<-streamDone

	// Verify events received.
	mu.Lock()
	defer mu.Unlock()

	// Filter events that match our VHTLC ID (ignore TxAssociated events and
	// events from other concurrent test operations).
	var htlcEvents []*pb.GetEventStreamResponse
	for _, ev := range events {
		switch e := ev.GetEvent().(type) {
		case *pb.GetEventStreamResponse_HtlcCreated:
			if e.HtlcCreated.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_created event: vhtlc_id=%s, address=%s",
					e.HtlcCreated.GetVhtlcId(), e.HtlcCreated.GetAddress())
			}
		case *pb.GetEventStreamResponse_HtlcFunded:
			if e.HtlcFunded.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_funded event: vhtlc_id=%s, funding_txid=%s",
					e.HtlcFunded.GetVhtlcId(), e.HtlcFunded.GetFundingTxid())
			}
		case *pb.GetEventStreamResponse_HtlcSpent:
			if e.HtlcSpent.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_spent event: vhtlc_id=%s, redeem_txid=%s, type=%s",
					e.HtlcSpent.GetVhtlcId(), e.HtlcSpent.GetRedeemTxid(), e.HtlcSpent.GetSpendType())
			}
		case *pb.GetEventStreamResponse_HtlcRefundable:
			if e.HtlcRefundable.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_refundable event: vhtlc_id=%s",
					e.HtlcRefundable.GetVhtlcId())
			}
		}
	}

	// We expect at least 2 HTLC events: htlc_created and htlc_spent.
	// (htlc_funded is only emitted through swap operations, not via direct
	// SendOffChain, so we do not require it in this test.)
	require.GreaterOrEqual(t, len(htlcEvents), 2,
		"expected at least 2 HTLC events (htlc_created + htlc_spent), got %d", len(htlcEvents))

	// Verify ordering: htlc_created must come before htlc_spent.
	var (
		foundCreated bool
		foundSpent   bool
		createdIdx   int
		spentIdx     int
	)
	for i, ev := range htlcEvents {
		switch ev.GetEvent().(type) {
		case *pb.GetEventStreamResponse_HtlcCreated:
			if !foundCreated {
				foundCreated = true
				createdIdx = i
			}
		case *pb.GetEventStreamResponse_HtlcSpent:
			if !foundSpent {
				foundSpent = true
				spentIdx = i
			}
		}
	}

	require.True(t, foundCreated, "expected htlc_created event for vhtlc_id %s", vhtlcId)
	require.True(t, foundSpent, "expected htlc_spent event for vhtlc_id %s", vhtlcId)
	require.Less(t, createdIdx, spentIdx,
		"htlc_created (idx %d) must come before htlc_spent (idx %d)", createdIdx, spentIdx)

	// Verify htlc_created event fields.
	createdEvent := htlcEvents[createdIdx].GetEvent().(*pb.GetEventStreamResponse_HtlcCreated)
	require.Equal(t, vhtlcId, createdEvent.HtlcCreated.GetVhtlcId())
	require.Equal(t, vhtlcResp.GetAddress(), createdEvent.HtlcCreated.GetAddress())
	require.NotZero(t, htlcEvents[createdIdx].GetTimestamp(), "htlc_created timestamp should be non-zero")

	// Verify htlc_spent event fields.
	spentEvent := htlcEvents[spentIdx].GetEvent().(*pb.GetEventStreamResponse_HtlcSpent)
	require.Equal(t, vhtlcId, spentEvent.HtlcSpent.GetVhtlcId())
	require.Equal(t, claimResp.GetRedeemTxid(), spentEvent.HtlcSpent.GetRedeemTxid())
	require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_CLAIMED, spentEvent.HtlcSpent.GetSpendType())
	require.NotZero(t, htlcEvents[spentIdx].GetTimestamp(), "htlc_spent timestamp should be non-zero")

	t.Log("all event stream assertions passed")
}

// TestEventStreamSubmarineSwap verifies that the GetEventStream endpoint
// delivers htlc_created and htlc_funded events during a submarine swap
// (PayInvoice), which internally creates a VHTLC and funds it through Boltz.
func TestEventStreamSubmarineSwap(t *testing.T) {
	invoiceAmount := 4500
	f, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	ctx := t.Context()

	// Verify sufficient balance.
	balance, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.Greater(t, int(balance.GetAmount()), invoiceAmount,
		"insufficient balance for submarine swap test")

	// Open the event stream.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := f.GetEventStream(streamCtx, &pb.GetEventStreamRequest{})
	require.NoError(t, err)

	// Collect events in background.
	var (
		mu     sync.Mutex
		events []*pb.GetEventStreamResponse
	)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}()

	// Perform submarine swap: create LND invoice, then pay it via Fulmine.
	invoice, _, err := lndAddInvoice(ctx, invoiceAmount)
	require.NoError(t, err)
	require.NotEmpty(t, invoice)

	payResp, err := f.PayInvoice(ctx, &pb.PayInvoiceRequest{
		Invoice: invoice,
	})
	if err != nil {
		// Submarine swaps depend on Boltz having Lightning routing available.
		// If Boltz cannot route, skip this test rather than fail hard because
		// the event stream itself is not at fault.
		if strings.Contains(err.Error(), "could not find route") {
			streamCancel()
			<-streamDone
			t.Skipf("skipping: Boltz cannot route payment (%v)", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, payResp)
	t.Logf("submarine swap completed, txid: %s", payResp.GetTxid())

	// Allow events to propagate.
	time.Sleep(5 * time.Second)

	// Close the stream.
	streamCancel()
	<-streamDone

	// Analyze events received.
	mu.Lock()
	defer mu.Unlock()

	t.Logf("total events received on stream: %d", len(events))
	for i, ev := range events {
		switch e := ev.GetEvent().(type) {
		case *pb.GetEventStreamResponse_HtlcCreated:
			t.Logf("event[%d] htlc_created: vhtlc_id=%s", i, e.HtlcCreated.GetVhtlcId())
		case *pb.GetEventStreamResponse_HtlcFunded:
			t.Logf("event[%d] htlc_funded: vhtlc_id=%s, funding_txid=%s, amount=%d",
				i, e.HtlcFunded.GetVhtlcId(), e.HtlcFunded.GetFundingTxid(), e.HtlcFunded.GetAmount())
		case *pb.GetEventStreamResponse_HtlcSpent:
			t.Logf("event[%d] htlc_spent: vhtlc_id=%s, type=%s",
				i, e.HtlcSpent.GetVhtlcId(), e.HtlcSpent.GetSpendType())
		case *pb.GetEventStreamResponse_HtlcRefundable:
			t.Logf("event[%d] htlc_refundable: vhtlc_id=%s",
				i, e.HtlcRefundable.GetVhtlcId())
		case *pb.GetEventStreamResponse_TxAssociated:
			t.Logf("event[%d] tx_associated: txid=%s", i, e.TxAssociated.GetTxid())
		}
	}

	// During a submarine swap, PayInvoice internally:
	//   1. Creates a VHTLC via Boltz (htlc_created emitted from GetSwapVHTLC)
	//   2. Funds the VHTLC (htlc_funded emitted from PayInvoice)
	// We check for htlc_funded because that confirms the swap pipeline emitted
	// the event. htlc_created may or may not arrive depending on timing
	// (it's emitted in a goroutine).
	var foundFunded bool
	for _, ev := range events {
		if funded := ev.GetHtlcFunded(); funded != nil {
			foundFunded = true
			require.NotEmpty(t, funded.GetVhtlcId(), "htlc_funded vhtlc_id should not be empty")
			require.NotEmpty(t, funded.GetFundingTxid(), "htlc_funded funding_txid should not be empty")
			require.NotZero(t, funded.GetAmount(), "htlc_funded amount should not be zero")
			require.NotZero(t, ev.GetTimestamp(), "htlc_funded timestamp should not be zero")
			t.Logf("verified htlc_funded: vhtlc_id=%s, funding_txid=%s, amount=%d",
				funded.GetVhtlcId(), funded.GetFundingTxid(), funded.GetAmount())
			break
		}
	}
	require.True(t, foundFunded,
		"expected at least one htlc_funded event during submarine swap")

	t.Log("submarine swap event stream assertions passed")
}

// TestEventStreamRefundVHTLC verifies that the GetEventStream endpoint
// delivers htlc_created and htlc_spent (REFUNDED) events when a VHTLC
// is created and then refunded.
func TestEventStreamRefundVHTLC(t *testing.T) {
	f, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)

	// Open the event stream.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := f.GetEventStream(streamCtx, &pb.GetEventStreamRequest{})
	require.NoError(t, err)

	var (
		mu     sync.Mutex
		events []*pb.GetEventStreamResponse
	)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}()

	// Generate preimage and hash.
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	// Use a different receiver key (not self) so we can refund.
	// Use a past refund locktime so the CLTV is already expired in regtest.
	pastRefundLocktime := uint32(1577836800) // Jan 1, 2020 00:00:00 UTC

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
		RefundLocktime: pastRefundLocktime,
		UnilateralClaimDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
	})
	require.NoError(t, err)
	vhtlcId := vhtlcResp.GetId()
	t.Logf("created VHTLC %s for refund test", vhtlcId)

	// Wait for htlc_created event to propagate.
	time.Sleep(2 * time.Second)

	// Fund the VHTLC.
	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.GetAddress(),
		Amount:  1000,
	})
	require.NoError(t, err)

	// Refund via SettleVHTLC with refund path.
	settleResp, err := f.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlcId,
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, settleResp)
	t.Logf("refunded VHTLC, txid: %s", settleResp.GetTxid())

	// Allow events to propagate.
	time.Sleep(3 * time.Second)

	streamCancel()
	<-streamDone

	mu.Lock()
	defer mu.Unlock()

	// Filter events for this VHTLC.
	var htlcEvents []*pb.GetEventStreamResponse
	for _, ev := range events {
		switch e := ev.GetEvent().(type) {
		case *pb.GetEventStreamResponse_HtlcCreated:
			if e.HtlcCreated.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_created: vhtlc_id=%s", e.HtlcCreated.GetVhtlcId())
			}
		case *pb.GetEventStreamResponse_HtlcSpent:
			if e.HtlcSpent.GetVhtlcId() == vhtlcId {
				htlcEvents = append(htlcEvents, ev)
				t.Logf("received htlc_spent: vhtlc_id=%s, type=%s",
					e.HtlcSpent.GetVhtlcId(), e.HtlcSpent.GetSpendType())
			}
		}
	}

	require.GreaterOrEqual(t, len(htlcEvents), 2,
		"expected at least 2 events (htlc_created + htlc_spent), got %d", len(htlcEvents))

	// Verify ordering and event types.
	var (
		foundCreated bool
		foundSpent   bool
		createdIdx   int
		spentIdx     int
	)
	for i, ev := range htlcEvents {
		switch ev.GetEvent().(type) {
		case *pb.GetEventStreamResponse_HtlcCreated:
			if !foundCreated {
				foundCreated = true
				createdIdx = i
			}
		case *pb.GetEventStreamResponse_HtlcSpent:
			if !foundSpent {
				foundSpent = true
				spentIdx = i
			}
		}
	}

	require.True(t, foundCreated, "expected htlc_created event")
	require.True(t, foundSpent, "expected htlc_spent event")
	require.Less(t, createdIdx, spentIdx, "htlc_created must come before htlc_spent")

	// Verify the spent event is of type REFUNDED.
	spentEvent := htlcEvents[spentIdx].GetEvent().(*pb.GetEventStreamResponse_HtlcSpent)
	require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_REFUNDED, spentEvent.HtlcSpent.GetSpendType(),
		"expected REFUNDED spend type for refund test")

	t.Log("refund event stream assertions passed")
}

// TestEventStreamAllEvents is a comprehensive integration test covering all 11
// event types delivered by the GetEventStream gRPC endpoint.
//
// It is split into two subtests:
//
//   - vhtlc_flow (runs against clientFulmineURL / real Boltz):
//     Round 1: htlc_created, htlc_funded (via PayInvoice), htlc_spent CLAIMED
//     Round 2: htlc_created, htlc_spent REFUNDED, htlc_refundable
//
//   - chain_swap_flow (runs against mockFulmineURL / mock-boltz):
//     Swap A (ARK_TO_BTC claim):   chain_swap_created, chain_swap_server_locked, chain_swap_claimed
//     Swap B (BTC_TO_ARK failure): chain_swap_created, chain_swap_user_locked, chain_swap_failed
//     Swap C (ARK_TO_BTC refund):  chain_swap_created, chain_swap_refunded COOPERATIVE
func TestEventStreamAllEvents(t *testing.T) {
	t.Run("vhtlc_flow", func(t *testing.T) {
		f, err := newFulmineClient(clientFulmineURL)
		require.NoError(t, err)

		ctx := t.Context()

		info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, info.GetPubkey())

		// ---- Round 1: htlc_created + htlc_funded + htlc_spent CLAIMED ----

		streamCtx1, cancel1 := context.WithCancel(ctx)
		defer cancel1()

		stream1, err := f.GetEventStream(streamCtx1, &pb.GetEventStreamRequest{})
		require.NoError(t, err)

		var (
			mu1     sync.Mutex
			events1 []*pb.GetEventStreamResponse
		)
		streamDone1 := make(chan struct{})
		go func() {
			defer close(streamDone1)
			for {
				ev, err := stream1.Recv()
				if err != nil {
					return
				}
				mu1.Lock()
				events1 = append(events1, ev)
				mu1.Unlock()
			}
		}()

		// Generate preimage for Round 1.
		preimage1 := make([]byte, 32)
		_, err = rand.Read(preimage1)
		require.NoError(t, err)
		sha256Hash1 := sha256.Sum256(preimage1)
		preimageHash1 := hex.EncodeToString(input.Ripemd160H(sha256Hash1[:]))

		// Step 1a: Create VHTLC (triggers htlc_created).
		vhtlcResp1, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
			PreimageHash:   preimageHash1,
			ReceiverPubkey: info.GetPubkey(),
			UnilateralClaimDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 512,
			},
			UnilateralRefundDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 512,
			},
			UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 1024,
			},
		})
		require.NoError(t, err)
		vhtlcId1 := vhtlcResp1.GetId()
		require.NotEmpty(t, vhtlcId1)
		t.Logf("round 1: created VHTLC %s", vhtlcId1)

		time.Sleep(2 * time.Second)

		// Step 1b: Fund the VHTLC (needed before ClaimVHTLC).
		_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
			Address: vhtlcResp1.GetAddress(),
			Amount:  1000,
		})
		require.NoError(t, err)
		t.Log("round 1: funded VHTLC via SendOffChain")

		// Step 1c: Claim the VHTLC (triggers htlc_spent CLAIMED).
		claimResp, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
			VhtlcId:  vhtlcId1,
			Preimage: hex.EncodeToString(preimage1),
		})
		require.NoError(t, err)
		require.NotEmpty(t, claimResp.GetRedeemTxid())
		t.Logf("round 1: claimed VHTLC, redeem txid: %s", claimResp.GetRedeemTxid())

		time.Sleep(3 * time.Second)

		// Step 1d: PayInvoice submarine swap (triggers htlc_funded on the internal VHTLC).
		invoice1, _, err := lndAddInvoice(ctx, 4500)
		require.NoError(t, err)

		payResp, err := f.PayInvoice(ctx, &pb.PayInvoiceRequest{
			Invoice: invoice1,
		})
		if err != nil {
			if strings.Contains(err.Error(), "could not find route") {
				cancel1()
				<-streamDone1
				t.Skipf("skipping vhtlc_flow: Boltz cannot route payment (%v)", err)
			}
			require.NoError(t, err)
		}
		require.NotNil(t, payResp)
		t.Logf("round 1: submarine swap completed, txid: %s", payResp.GetTxid())

		time.Sleep(5 * time.Second)

		// Close Round 1 stream.
		cancel1()
		<-streamDone1

		// Round 1 assertions.
		mu1.Lock()

		// Verify htlc_created + htlc_spent for our explicit VHTLC (vhtlcId1).
		var (
			foundCreated1 bool
			foundSpent1   bool
			createdIdx1   int
			spentIdx1     int
		)
		for i, ev := range events1 {
			switch e := ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_HtlcCreated:
				if e.HtlcCreated.GetVhtlcId() == vhtlcId1 && !foundCreated1 {
					foundCreated1 = true
					createdIdx1 = i
					t.Logf("round 1 event: htlc_created vhtlc_id=%s", e.HtlcCreated.GetVhtlcId())
				}
			case *pb.GetEventStreamResponse_HtlcSpent:
				if e.HtlcSpent.GetVhtlcId() == vhtlcId1 && !foundSpent1 {
					foundSpent1 = true
					spentIdx1 = i
					t.Logf("round 1 event: htlc_spent vhtlc_id=%s type=%s",
						e.HtlcSpent.GetVhtlcId(), e.HtlcSpent.GetSpendType())
				}
			}
		}
		require.True(t, foundCreated1, "round 1: expected htlc_created for vhtlc_id %s", vhtlcId1)
		require.True(t, foundSpent1, "round 1: expected htlc_spent for vhtlc_id %s", vhtlcId1)
		require.Less(t, createdIdx1, spentIdx1, "round 1: htlc_created must precede htlc_spent")

		spentEvent1 := events1[spentIdx1].GetEvent().(*pb.GetEventStreamResponse_HtlcSpent)
		require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_CLAIMED, spentEvent1.HtlcSpent.GetSpendType(),
			"round 1: expected CLAIMED spend type")
		require.Equal(t, claimResp.GetRedeemTxid(), spentEvent1.HtlcSpent.GetRedeemTxid(),
			"round 1: redeem txid mismatch")
		require.NotZero(t, events1[createdIdx1].GetTimestamp(), "round 1: htlc_created timestamp zero")
		require.NotZero(t, events1[spentIdx1].GetTimestamp(), "round 1: htlc_spent timestamp zero")

		// Verify htlc_funded event from PayInvoice (no ID filter — PayInvoice creates
		// its own internal VHTLC via Boltz).
		var foundFunded bool
		for _, ev := range events1 {
			if funded := ev.GetHtlcFunded(); funded != nil {
				foundFunded = true
				require.NotEmpty(t, funded.GetVhtlcId(), "htlc_funded: vhtlc_id empty")
				require.NotEmpty(t, funded.GetFundingTxid(), "htlc_funded: funding_txid empty")
				require.NotZero(t, funded.GetAmount(), "htlc_funded: amount zero")
				require.NotZero(t, ev.GetTimestamp(), "htlc_funded: timestamp zero")
				t.Logf("round 1 event: htlc_funded vhtlc_id=%s funding_txid=%s amount=%d",
					funded.GetVhtlcId(), funded.GetFundingTxid(), funded.GetAmount())
				break
			}
		}
		require.True(t, foundFunded, "round 1: expected htlc_funded event from PayInvoice")

		mu1.Unlock()

		t.Log("round 1 assertions passed: htlc_created, htlc_funded, htlc_spent CLAIMED")

		// ---- Round 2: htlc_created + htlc_spent REFUNDED + htlc_refundable ----

		streamCtx2, cancel2 := context.WithCancel(ctx)
		defer cancel2()

		stream2, err := f.GetEventStream(streamCtx2, &pb.GetEventStreamRequest{})
		require.NoError(t, err)

		var (
			mu2     sync.Mutex
			events2 []*pb.GetEventStreamResponse
		)
		streamDone2 := make(chan struct{})
		go func() {
			defer close(streamDone2)
			for {
				ev, err := stream2.Recv()
				if err != nil {
					return
				}
				mu2.Lock()
				events2 = append(events2, ev)
				mu2.Unlock()
			}
		}()

		// Generate preimage for Round 2.
		preimage2 := make([]byte, 32)
		_, err = rand.Read(preimage2)
		require.NoError(t, err)
		sha256Hash2 := sha256.Sum256(preimage2)
		preimageHash2 := hex.EncodeToString(input.Ripemd160H(sha256Hash2[:]))

		// Step 2a: Create VHTLC with past refund locktime (triggers htlc_created +
		// htlc_refundable from the background watcher).
		pastRefundLocktime := uint32(1577836800) // Jan 1, 2020 — already expired in regtest.
		vhtlcResp2, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
			PreimageHash:   preimageHash2,
			ReceiverPubkey: info.GetPubkey(),
			RefundLocktime: pastRefundLocktime,
			UnilateralClaimDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 512,
			},
			UnilateralRefundDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 512,
			},
			UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
				Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
				Value: 512,
			},
		})
		require.NoError(t, err)
		vhtlcId2 := vhtlcResp2.GetId()
		require.NotEmpty(t, vhtlcId2)
		t.Logf("round 2: created VHTLC %s (past locktime)", vhtlcId2)

		time.Sleep(2 * time.Second)

		// Step 2b: Fund the VHTLC.
		_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
			Address: vhtlcResp2.GetAddress(),
			Amount:  1000,
		})
		require.NoError(t, err)
		t.Log("round 2: funded VHTLC via SendOffChain")

		// Step 2c: Refund via SettleVHTLC (triggers htlc_spent REFUNDED).
		settleResp, err := f.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
			VhtlcId: vhtlcId2,
			SettlementType: &pb.SettleVHTLCRequest_Refund{
				Refund: &pb.RefundPath{},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, settleResp)
		t.Logf("round 2: refunded VHTLC, txid: %s", settleResp.GetTxid())

		// Wait for events including htlc_refundable from the background watcher.
		time.Sleep(5 * time.Second)

		cancel2()
		<-streamDone2

		// Round 2 assertions.
		mu2.Lock()

		var (
			foundCreated2    bool
			foundSpent2      bool
			foundRefundable2 bool
		)
		for _, ev := range events2 {
			switch e := ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_HtlcCreated:
				if e.HtlcCreated.GetVhtlcId() == vhtlcId2 {
					foundCreated2 = true
					require.NotZero(t, ev.GetTimestamp(), "round 2: htlc_created timestamp zero")
					t.Logf("round 2 event: htlc_created vhtlc_id=%s", e.HtlcCreated.GetVhtlcId())
				}
			case *pb.GetEventStreamResponse_HtlcSpent:
				if e.HtlcSpent.GetVhtlcId() == vhtlcId2 {
					foundSpent2 = true
					require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_REFUNDED, e.HtlcSpent.GetSpendType(),
						"round 2: expected REFUNDED spend type")
					require.NotZero(t, ev.GetTimestamp(), "round 2: htlc_spent timestamp zero")
					t.Logf("round 2 event: htlc_spent vhtlc_id=%s type=%s",
						e.HtlcSpent.GetVhtlcId(), e.HtlcSpent.GetSpendType())
				}
			case *pb.GetEventStreamResponse_HtlcRefundable:
				if e.HtlcRefundable.GetVhtlcId() == vhtlcId2 {
					foundRefundable2 = true
					require.NotZero(t, ev.GetTimestamp(), "round 2: htlc_refundable timestamp zero")
					t.Logf("round 2 event: htlc_refundable vhtlc_id=%s", e.HtlcRefundable.GetVhtlcId())
				}
			}
		}

		require.True(t, foundCreated2, "round 2: expected htlc_created for vhtlc_id %s", vhtlcId2)
		require.True(t, foundSpent2, "round 2: expected htlc_spent REFUNDED for vhtlc_id %s", vhtlcId2)
		require.True(t, foundRefundable2, "round 2: expected htlc_refundable for vhtlc_id %s", vhtlcId2)

		mu2.Unlock()

		t.Log("round 2 assertions passed: htlc_created, htlc_spent REFUNDED, htlc_refundable")
		t.Log("vhtlc_flow: all 5 HTLC event types verified")
	})

	t.Run("chain_swap_flow", func(t *testing.T) {
		client, err := newFulmineClient(mockFulmineURL)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
		defer cancel()

		// Open a single event stream for all three mini-swaps.
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()

		stream, err := client.GetEventStream(streamCtx, &pb.GetEventStreamRequest{})
		require.NoError(t, err)

		var (
			mu     sync.Mutex
			events []*pb.GetEventStreamResponse
		)
		streamDone := make(chan struct{})
		go func() {
			defer close(streamDone)
			for {
				ev, err := stream.Recv()
				if err != nil {
					return
				}
				mu.Lock()
				events = append(events, ev)
				mu.Unlock()
			}
		}()

		// filterBySwap extracts chain swap events matching a specific swap ID.
		filterBySwap := func(swapID string) []*pb.GetEventStreamResponse {
			mu.Lock()
			defer mu.Unlock()
			var out []*pb.GetEventStreamResponse
			for _, ev := range events {
				switch e := ev.GetEvent().(type) {
				case *pb.GetEventStreamResponse_ChainSwapCreated:
					if e.ChainSwapCreated.GetSwapId() == swapID {
						out = append(out, ev)
					}
				case *pb.GetEventStreamResponse_ChainSwapServerLocked:
					if e.ChainSwapServerLocked.GetSwapId() == swapID {
						out = append(out, ev)
					}
				case *pb.GetEventStreamResponse_ChainSwapUserLocked:
					if e.ChainSwapUserLocked.GetSwapId() == swapID {
						out = append(out, ev)
					}
				case *pb.GetEventStreamResponse_ChainSwapClaimed:
					if e.ChainSwapClaimed.GetSwapId() == swapID {
						out = append(out, ev)
					}
				case *pb.GetEventStreamResponse_ChainSwapRefunded:
					if e.ChainSwapRefunded.GetSwapId() == swapID {
						out = append(out, ev)
					}
				case *pb.GetEventStreamResponse_ChainSwapFailed:
					if e.ChainSwapFailed.GetSwapId() == swapID {
						out = append(out, ev)
					}
				}
			}
			return out
		}

		// ---- Swap A: ARK_TO_BTC script-path claim ----
		// Expected events: chain_swap_created, chain_swap_server_locked, chain_swap_claimed
		t.Log("swap A: starting ARK_TO_BTC claim flow")

		mockReset(t)
		mockSetConfig(t, map[string]any{"claimMode": "fail", "refundMode": "success"})

		btcAddrA := nigiriGetNewAddress(t, ctx)
		respA, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddrA,
		})
		require.NoError(t, err)
		swapA := respA.GetId()
		require.NotEmpty(t, swapA)
		t.Logf("swap A: created %s", swapA)

		stateA := mockGetSwap(t, swapA)
		require.NotEmpty(t, stateA.BTCLockupAddress)
		require.Greater(t, stateA.ServerLockAmount, uint64(0))

		txidA, txhexA := fundAddressAndGetConfirmedTx(t, ctx, stateA.BTCLockupAddress, stateA.ServerLockAmount)

		time.Sleep(1 * time.Second)
		mockPushEvent(t, swapA, "transaction.confirmed")
		mockPushEventWithTx(t, swapA, "transaction.server.mempool", txidA, txhexA)

		waitChainSwapStatus(t, ctx, client, swapA, "claimed", 45*time.Second)
		time.Sleep(2 * time.Second)

		// Assertions for Swap A.
		eventsA := filterBySwap(swapA)
		t.Logf("swap A: received %d events", len(eventsA))
		for i, ev := range eventsA {
			switch ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				t.Logf("swap A event[%d]: chain_swap_created", i)
			case *pb.GetEventStreamResponse_ChainSwapServerLocked:
				t.Logf("swap A event[%d]: chain_swap_server_locked", i)
			case *pb.GetEventStreamResponse_ChainSwapClaimed:
				t.Logf("swap A event[%d]: chain_swap_claimed", i)
			default:
				t.Logf("swap A event[%d]: other (%T)", i, ev.GetEvent())
			}
		}

		var (
			foundSwapACreated      bool
			foundSwapAServerLocked bool
			foundSwapAClaimed      bool
		)
		for _, ev := range eventsA {
			require.NotZero(t, ev.GetTimestamp(), "swap A: event timestamp zero")
			switch e := ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				foundSwapACreated = true
				require.Equal(t, swapA, e.ChainSwapCreated.GetSwapId())
				require.NotEmpty(t, e.ChainSwapCreated.GetDirection())
				require.NotZero(t, e.ChainSwapCreated.GetAmount())
			case *pb.GetEventStreamResponse_ChainSwapServerLocked:
				foundSwapAServerLocked = true
				require.Equal(t, swapA, e.ChainSwapServerLocked.GetSwapId())
				require.NotEmpty(t, e.ChainSwapServerLocked.GetServerLockupTxid())
			case *pb.GetEventStreamResponse_ChainSwapClaimed:
				foundSwapAClaimed = true
				require.Equal(t, swapA, e.ChainSwapClaimed.GetSwapId())
				require.NotEmpty(t, e.ChainSwapClaimed.GetClaimTxid())
			}
		}
		require.True(t, foundSwapACreated, "swap A: expected chain_swap_created")
		require.True(t, foundSwapAServerLocked, "swap A: expected chain_swap_server_locked")
		require.True(t, foundSwapAClaimed, "swap A: expected chain_swap_claimed")

		t.Log("swap A assertions passed: chain_swap_created, chain_swap_server_locked, chain_swap_claimed")

		// ---- Swap B: BTC_TO_ARK failure ----
		// Expected events: chain_swap_created, chain_swap_user_locked, chain_swap_failed
		t.Log("swap B: starting BTC_TO_ARK failure flow")

		mockReset(t)

		respB, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
			Amount:    3000,
		})
		require.NoError(t, err)
		swapB := respB.GetId()
		require.NotEmpty(t, swapB)
		t.Logf("swap B: created %s", swapB)

		userLockTxID, userLockTxHex := fundAddressAndGetConfirmedTx(
			t, ctx, respB.GetLockupAddress(), respB.GetExpectedAmount(),
		)

		mockPushEventWithTx(t, swapB, "transaction.confirmed", userLockTxID, userLockTxHex)
		time.Sleep(2 * time.Second)

		mockPushEvent(t, swapB, "transaction.failed")
		time.Sleep(3 * time.Second)

		// Assertions for Swap B.
		eventsB := filterBySwap(swapB)
		t.Logf("swap B: received %d events", len(eventsB))
		for i, ev := range eventsB {
			switch ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				t.Logf("swap B event[%d]: chain_swap_created", i)
			case *pb.GetEventStreamResponse_ChainSwapUserLocked:
				t.Logf("swap B event[%d]: chain_swap_user_locked", i)
			case *pb.GetEventStreamResponse_ChainSwapFailed:
				t.Logf("swap B event[%d]: chain_swap_failed", i)
			default:
				t.Logf("swap B event[%d]: other (%T)", i, ev.GetEvent())
			}
		}

		var (
			foundSwapBCreated    bool
			foundSwapBUserLocked bool
			foundSwapBFailed     bool
		)
		for _, ev := range eventsB {
			require.NotZero(t, ev.GetTimestamp(), "swap B: event timestamp zero")
			switch e := ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				foundSwapBCreated = true
				require.Equal(t, swapB, e.ChainSwapCreated.GetSwapId())
			case *pb.GetEventStreamResponse_ChainSwapUserLocked:
				foundSwapBUserLocked = true
				require.Equal(t, swapB, e.ChainSwapUserLocked.GetSwapId())
				require.NotEmpty(t, e.ChainSwapUserLocked.GetUserLockupTxid())
			case *pb.GetEventStreamResponse_ChainSwapFailed:
				foundSwapBFailed = true
				require.Equal(t, swapB, e.ChainSwapFailed.GetSwapId())
				require.NotEmpty(t, e.ChainSwapFailed.GetErrorMessage())
			}
		}
		require.True(t, foundSwapBCreated, "swap B: expected chain_swap_created")
		require.True(t, foundSwapBUserLocked, "swap B: expected chain_swap_user_locked")
		require.True(t, foundSwapBFailed, "swap B: expected chain_swap_failed")

		t.Log("swap B assertions passed: chain_swap_created, chain_swap_user_locked, chain_swap_failed")

		// ---- Swap C: ARK_TO_BTC cooperative refund ----
		// Expected events: chain_swap_created, chain_swap_refunded COOPERATIVE
		t.Log("swap C: starting ARK_TO_BTC cooperative refund flow")

		mockReset(t)
		mockSetConfig(t, map[string]any{"refundMode": "success"})

		btcAddrC := nigiriGetNewAddress(t, ctx)
		respC, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
			Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
			Amount:     3000,
			BtcAddress: btcAddrC,
		})
		require.NoError(t, err)
		swapC := respC.GetId()
		require.NotEmpty(t, swapC)
		t.Logf("swap C: created %s", swapC)

		time.Sleep(2 * time.Second)

		mockPushEvent(t, swapC, "swap.expired")
		waitChainSwapStatus(t, ctx, client, swapC, "refunded", 45*time.Second)
		time.Sleep(3 * time.Second)

		// Drain the stream.
		streamCancel()
		<-streamDone

		// Assertions for Swap C.
		eventsC := filterBySwap(swapC)
		t.Logf("swap C: received %d events", len(eventsC))
		for i, ev := range eventsC {
			switch ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				t.Logf("swap C event[%d]: chain_swap_created", i)
			case *pb.GetEventStreamResponse_ChainSwapRefunded:
				t.Logf("swap C event[%d]: chain_swap_refunded", i)
			default:
				t.Logf("swap C event[%d]: other (%T)", i, ev.GetEvent())
			}
		}

		var (
			foundSwapCCreated  bool
			foundSwapCRefunded bool
		)
		for _, ev := range eventsC {
			require.NotZero(t, ev.GetTimestamp(), "swap C: event timestamp zero")
			switch e := ev.GetEvent().(type) {
			case *pb.GetEventStreamResponse_ChainSwapCreated:
				foundSwapCCreated = true
				require.Equal(t, swapC, e.ChainSwapCreated.GetSwapId())
			case *pb.GetEventStreamResponse_ChainSwapRefunded:
				foundSwapCRefunded = true
				require.Equal(t, swapC, e.ChainSwapRefunded.GetSwapId())
				require.NotEmpty(t, e.ChainSwapRefunded.GetRefundTxid())
				require.Equal(t, pb.ChainSwapRefundedEvent_REFUND_KIND_COOPERATIVE,
					e.ChainSwapRefunded.GetKind(),
					"swap C: expected COOPERATIVE refund kind")
			}
		}
		require.True(t, foundSwapCCreated, "swap C: expected chain_swap_created")
		require.True(t, foundSwapCRefunded, "swap C: expected chain_swap_refunded COOPERATIVE")

		t.Log("swap C assertions passed: chain_swap_created, chain_swap_refunded COOPERATIVE")
		t.Log("chain_swap_flow: all 6 chain swap event types verified")
	})
}
