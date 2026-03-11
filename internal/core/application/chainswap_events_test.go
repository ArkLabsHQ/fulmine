package application

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEmitChainSwapEvent_NoConsumer(t *testing.T) {
	ch := make(chan ChainSwapEvent, 1024)
	svc := &Service{chainSwapEvents: ch}

	// Emit without any consumer; should not block or panic
	svc.emitChainSwapEvent(ChainSwapEvent{
		Type:      ChainSwapEventCreated,
		Timestamp: time.Now().Unix(),
		SwapId:    "test-swap-id",
	})

	// Verify event was buffered
	select {
	case e := <-ch:
		require.Equal(t, ChainSwapEventCreated, e.Type)
		require.Equal(t, "test-swap-id", e.SwapId)
	default:
		t.Fatal("expected event in channel")
	}
}

func TestEmitChainSwapEvent_ChannelFull(t *testing.T) {
	// Use a buffer size of 1 to easily fill it
	ch := make(chan ChainSwapEvent, 1)
	svc := &Service{chainSwapEvents: ch}

	// Fill the channel
	svc.emitChainSwapEvent(ChainSwapEvent{
		Type:   ChainSwapEventCreated,
		SwapId: "first",
	})

	// Second emit should drop without blocking
	done := make(chan struct{})
	go func() {
		svc.emitChainSwapEvent(ChainSwapEvent{
			Type:   ChainSwapEventClaimed,
			SwapId: "second",
		})
		close(done)
	}()

	select {
	case <-done:
		// Good, did not block
	case <-time.After(time.Second):
		t.Fatal("emitChainSwapEvent blocked when channel was full")
	}

	// Only the first event should be in the channel
	e := <-ch
	require.Equal(t, "first", e.SwapId)
}

func TestGetChainSwapEvents_ReturnsChannel(t *testing.T) {
	ch := make(chan ChainSwapEvent, 1024)
	svc := &Service{chainSwapEvents: ch}

	result := svc.GetChainSwapEvents(nil)
	require.NotNil(t, result)

	// Verify it's the same channel
	svc.emitChainSwapEvent(ChainSwapEvent{
		Type:   ChainSwapEventFailed,
		SwapId: "test-channel",
	})

	select {
	case e := <-result:
		require.Equal(t, ChainSwapEventFailed, e.Type)
		require.Equal(t, "test-channel", e.SwapId)
	case <-time.After(time.Second):
		t.Fatal("did not receive event from GetChainSwapEvents channel")
	}
}

func TestChainSwapEvent_AllTypes(t *testing.T) {
	ch := make(chan ChainSwapEvent, 1024)
	svc := &Service{chainSwapEvents: ch}

	tests := []struct {
		name  string
		event ChainSwapEvent
	}{
		{
			name: "chain_swap_created",
			event: ChainSwapEvent{
				Type:      ChainSwapEventCreated,
				Timestamp: time.Now().Unix(),
				SwapId:    "created-id",
				Direction: "ark_to_btc",
				Amount:    50000,
			},
		},
		{
			name: "chain_swap_user_locked",
			event: ChainSwapEvent{
				Type:         ChainSwapEventUserLocked,
				Timestamp:    time.Now().Unix(),
				SwapId:       "userlocked-id",
				UserLockTxId: "user-lock-txid-123",
			},
		},
		{
			name: "chain_swap_server_locked",
			event: ChainSwapEvent{
				Type:           ChainSwapEventServerLocked,
				Timestamp:      time.Now().Unix(),
				SwapId:         "serverlocked-id",
				ServerLockTxId: "server-lock-txid-456",
			},
		},
		{
			name: "chain_swap_claimed",
			event: ChainSwapEvent{
				Type:      ChainSwapEventClaimed,
				Timestamp: time.Now().Unix(),
				SwapId:    "claimed-id",
				ClaimTxId: "claim-txid-789",
			},
		},
		{
			name: "chain_swap_refunded_cooperative",
			event: ChainSwapEvent{
				Type:       ChainSwapEventRefunded,
				Timestamp:  time.Now().Unix(),
				SwapId:     "refunded-coop-id",
				RefundTxId: "refund-txid-coop",
				RefundKind: ChainSwapRefundCooperative,
			},
		},
		{
			name: "chain_swap_refunded_unilateral",
			event: ChainSwapEvent{
				Type:       ChainSwapEventRefunded,
				Timestamp:  time.Now().Unix(),
				SwapId:     "refunded-uni-id",
				RefundTxId: "refund-txid-uni",
				RefundKind: ChainSwapRefundUnilateral,
			},
		},
		{
			name: "chain_swap_failed",
			event: ChainSwapEvent{
				Type:         ChainSwapEventFailed,
				Timestamp:    time.Now().Unix(),
				SwapId:       "failed-id",
				ErrorMessage: "something went wrong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.emitChainSwapEvent(tt.event)

			select {
			case e := <-ch:
				require.Equal(t, tt.event.Type, e.Type)
				require.Equal(t, tt.event.SwapId, e.SwapId)
				require.Equal(t, tt.event.Direction, e.Direction)
				require.Equal(t, tt.event.Amount, e.Amount)
				require.Equal(t, tt.event.UserLockTxId, e.UserLockTxId)
				require.Equal(t, tt.event.ServerLockTxId, e.ServerLockTxId)
				require.Equal(t, tt.event.ClaimTxId, e.ClaimTxId)
				require.Equal(t, tt.event.RefundTxId, e.RefundTxId)
				require.Equal(t, tt.event.RefundKind, e.RefundKind)
				require.Equal(t, tt.event.ErrorMessage, e.ErrorMessage)
			case <-time.After(time.Second):
				t.Fatal("did not receive event")
			}
		})
	}
}
