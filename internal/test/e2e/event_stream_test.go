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
