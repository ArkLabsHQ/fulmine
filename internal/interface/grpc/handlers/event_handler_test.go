package handlers

import (
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/stretchr/testify/require"
)

func TestToEventStreamResponse_HtlcCreated(t *testing.T) {
	event := application.HtlcEvent{
		Type:      application.HtlcEventCreated,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "abc123",
		Address:   "tark1abc",
		Amount:    50000,
	}

	resp := toEventStreamResponse(event)
	require.NotNil(t, resp)
	require.Equal(t, event.Timestamp, resp.Timestamp)

	created := resp.GetHtlcCreated()
	require.NotNil(t, created)
	require.Equal(t, "abc123", created.VhtlcId)
	require.Equal(t, "tark1abc", created.Address)
	require.Equal(t, uint64(50000), created.Amount)
}

func TestToEventStreamResponse_HtlcFunded(t *testing.T) {
	event := application.HtlcEvent{
		Type:      application.HtlcEventFunded,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "funded-id",
		TxId:      "txid123",
		Amount:    100000,
	}

	resp := toEventStreamResponse(event)
	require.NotNil(t, resp)

	funded := resp.GetHtlcFunded()
	require.NotNil(t, funded)
	require.Equal(t, "funded-id", funded.VhtlcId)
	require.Equal(t, "txid123", funded.FundingTxid)
	require.Equal(t, uint64(100000), funded.Amount)
}

func TestToEventStreamResponse_HtlcSpentClaimed(t *testing.T) {
	event := application.HtlcEvent{
		Type:      application.HtlcEventSpent,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "spent-id",
		TxId:      "redeem123",
		SpendKind: application.SpendTypeClaimed,
	}

	resp := toEventStreamResponse(event)
	require.NotNil(t, resp)

	spent := resp.GetHtlcSpent()
	require.NotNil(t, spent)
	require.Equal(t, "spent-id", spent.VhtlcId)
	require.Equal(t, "redeem123", spent.RedeemTxid)
	require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_CLAIMED, spent.SpendType)
}

func TestToEventStreamResponse_HtlcSpentRefunded(t *testing.T) {
	event := application.HtlcEvent{
		Type:      application.HtlcEventSpent,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "refund-id",
		TxId:      "refund-txid",
		SpendKind: application.SpendTypeRefunded,
	}

	resp := toEventStreamResponse(event)
	require.NotNil(t, resp)

	spent := resp.GetHtlcSpent()
	require.NotNil(t, spent)
	require.Equal(t, "refund-id", spent.VhtlcId)
	require.Equal(t, "refund-txid", spent.RedeemTxid)
	require.Equal(t, pb.HtlcSpentEvent_SPEND_TYPE_REFUNDED, spent.SpendType)
}

func TestToEventStreamResponse_HtlcRefundable(t *testing.T) {
	event := application.HtlcEvent{
		Type:           application.HtlcEventRefundable,
		Timestamp:      time.Now().Unix(),
		VhtlcId:        "refundable-id",
		RefundLocktime: 800000,
	}

	resp := toEventStreamResponse(event)
	require.NotNil(t, resp)

	refundable := resp.GetHtlcRefundable()
	require.NotNil(t, refundable)
	require.Equal(t, "refundable-id", refundable.VhtlcId)
	require.Equal(t, uint64(800000), refundable.RefundLocktime)
}

func TestEventListenerHandler_PushAndRemove(t *testing.T) {
	handler := newListenerHandler[*pb.GetEventStreamResponse]()

	l1 := &listener[*pb.GetEventStreamResponse]{
		id: "l1",
		ch: make(chan *pb.GetEventStreamResponse, 1),
	}
	l2 := &listener[*pb.GetEventStreamResponse]{
		id: "l2",
		ch: make(chan *pb.GetEventStreamResponse, 1),
	}

	handler.pushListener(l1)
	handler.pushListener(l2)
	require.Len(t, handler.listeners, 2)

	handler.removeListener("l1")
	require.Len(t, handler.listeners, 1)
	require.Equal(t, "l2", handler.listeners[0].id)
}

func TestEventFanOut_MultipleConsumers(t *testing.T) {
	handler := newListenerHandler[*pb.GetEventStreamResponse]()

	l1 := &listener[*pb.GetEventStreamResponse]{
		id: "l1",
		ch: make(chan *pb.GetEventStreamResponse, 10),
	}
	l2 := &listener[*pb.GetEventStreamResponse]{
		id: "l2",
		ch: make(chan *pb.GetEventStreamResponse, 10),
	}

	handler.pushListener(l1)
	handler.pushListener(l2)

	event := toEventStreamResponse(application.HtlcEvent{
		Type:      application.HtlcEventCreated,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "fanout-test",
	})

	// Simulate fan-out (same pattern as listenToHtlcEvents)
	handler.lock.Lock()
	for _, l := range handler.listeners {
		go func(l *listener[*pb.GetEventStreamResponse]) {
			l.ch <- event
		}(l)
	}
	handler.lock.Unlock()

	// Both listeners should receive the event
	select {
	case e := <-l1.ch:
		require.Equal(t, "fanout-test", e.GetHtlcCreated().VhtlcId)
	case <-time.After(time.Second):
		t.Fatal("l1 did not receive event")
	}

	select {
	case e := <-l2.ch:
		require.Equal(t, "fanout-test", e.GetHtlcCreated().VhtlcId)
	case <-time.After(time.Second):
		t.Fatal("l2 did not receive event")
	}
}

func TestToChainSwapEventStreamResponse_Created(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:      application.ChainSwapEventCreated,
		Timestamp: time.Now().Unix(),
		SwapId:    "swap-123",
		Direction: "ark_to_btc",
		Amount:    50000,
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)
	require.Equal(t, event.Timestamp, resp.Timestamp)

	created := resp.GetChainSwapCreated()
	require.NotNil(t, created)
	require.Equal(t, "swap-123", created.SwapId)
	require.Equal(t, "ark_to_btc", created.Direction)
	require.Equal(t, uint64(50000), created.Amount)
}

func TestToChainSwapEventStreamResponse_UserLocked(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:         application.ChainSwapEventUserLocked,
		Timestamp:    time.Now().Unix(),
		SwapId:       "swap-456",
		UserLockTxId: "user-lock-txid",
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	userLocked := resp.GetChainSwapUserLocked()
	require.NotNil(t, userLocked)
	require.Equal(t, "swap-456", userLocked.SwapId)
	require.Equal(t, "user-lock-txid", userLocked.UserLockupTxid)
}

func TestToChainSwapEventStreamResponse_ServerLocked(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:           application.ChainSwapEventServerLocked,
		Timestamp:      time.Now().Unix(),
		SwapId:         "swap-789",
		ServerLockTxId: "server-lock-txid",
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	serverLocked := resp.GetChainSwapServerLocked()
	require.NotNil(t, serverLocked)
	require.Equal(t, "swap-789", serverLocked.SwapId)
	require.Equal(t, "server-lock-txid", serverLocked.ServerLockupTxid)
}

func TestToChainSwapEventStreamResponse_Claimed(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:      application.ChainSwapEventClaimed,
		Timestamp: time.Now().Unix(),
		SwapId:    "swap-claimed",
		ClaimTxId: "claim-txid-abc",
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	claimed := resp.GetChainSwapClaimed()
	require.NotNil(t, claimed)
	require.Equal(t, "swap-claimed", claimed.SwapId)
	require.Equal(t, "claim-txid-abc", claimed.ClaimTxid)
}

func TestToChainSwapEventStreamResponse_Refunded_Cooperative(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:       application.ChainSwapEventRefunded,
		Timestamp:  time.Now().Unix(),
		SwapId:     "swap-refund-coop",
		RefundTxId: "refund-txid-coop",
		RefundKind: application.ChainSwapRefundCooperative,
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	refunded := resp.GetChainSwapRefunded()
	require.NotNil(t, refunded)
	require.Equal(t, "swap-refund-coop", refunded.SwapId)
	require.Equal(t, "refund-txid-coop", refunded.RefundTxid)
	require.Equal(t, pb.ChainSwapRefundedEvent_REFUND_KIND_COOPERATIVE, refunded.Kind)
}

func TestToChainSwapEventStreamResponse_Refunded_Unilateral(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:       application.ChainSwapEventRefunded,
		Timestamp:  time.Now().Unix(),
		SwapId:     "swap-refund-uni",
		RefundTxId: "refund-txid-uni",
		RefundKind: application.ChainSwapRefundUnilateral,
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	refunded := resp.GetChainSwapRefunded()
	require.NotNil(t, refunded)
	require.Equal(t, "swap-refund-uni", refunded.SwapId)
	require.Equal(t, "refund-txid-uni", refunded.RefundTxid)
	require.Equal(t, pb.ChainSwapRefundedEvent_REFUND_KIND_UNILATERAL, refunded.Kind)
}

func TestToChainSwapEventStreamResponse_Failed(t *testing.T) {
	event := application.ChainSwapEvent{
		Type:         application.ChainSwapEventFailed,
		Timestamp:    time.Now().Unix(),
		SwapId:       "swap-failed",
		ErrorMessage: "something went wrong",
	}

	resp := toChainSwapEventStreamResponse(event)
	require.NotNil(t, resp)

	failed := resp.GetChainSwapFailed()
	require.NotNil(t, failed)
	require.Equal(t, "swap-failed", failed.SwapId)
	require.Equal(t, "something went wrong", failed.ErrorMessage)
}

func TestTxAssociatedResponse(t *testing.T) {
	notif := application.Notification{}
	notif.Txid = "tx-assoc-id"
	notif.Tx = "rawtx"

	resp := toTxAssociatedResponse(notif)
	require.NotNil(t, resp)
	require.NotZero(t, resp.Timestamp)

	txAssoc := resp.GetTxAssociated()
	require.NotNil(t, txAssoc)
	require.Equal(t, "tx-assoc-id", txAssoc.Txid)
	require.Equal(t, "rawtx", txAssoc.Tx)
}
