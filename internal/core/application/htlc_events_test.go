package application

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEmitHtlcEvent_NoConsumer(t *testing.T) {
	ch := make(chan HtlcEvent, 1024)
	svc := &Service{htlcEvents: ch}

	// Emit without any consumer; should not block or panic
	svc.emitHtlcEvent(HtlcEvent{
		Type:      HtlcEventCreated,
		Timestamp: time.Now().Unix(),
		VhtlcId:   "test-id",
	})

	// Verify event was buffered
	select {
	case e := <-ch:
		require.Equal(t, HtlcEventCreated, e.Type)
		require.Equal(t, "test-id", e.VhtlcId)
	default:
		t.Fatal("expected event in channel")
	}
}

func TestEmitHtlcEvent_ChannelFull(t *testing.T) {
	// Use a buffer size of 1 to easily fill it
	ch := make(chan HtlcEvent, 1)
	svc := &Service{htlcEvents: ch}

	// Fill the channel
	svc.emitHtlcEvent(HtlcEvent{
		Type:    HtlcEventCreated,
		VhtlcId: "first",
	})

	// Second emit should drop without blocking
	done := make(chan struct{})
	go func() {
		svc.emitHtlcEvent(HtlcEvent{
			Type:    HtlcEventFunded,
			VhtlcId: "second",
		})
		close(done)
	}()

	select {
	case <-done:
		// Good, did not block
	case <-time.After(time.Second):
		t.Fatal("emitHtlcEvent blocked when channel was full")
	}

	// Only the first event should be in the channel
	e := <-ch
	require.Equal(t, "first", e.VhtlcId)
}

func TestGetHtlcEvents_ReturnsChannel(t *testing.T) {
	ch := make(chan HtlcEvent, 1024)
	svc := &Service{htlcEvents: ch}

	result := svc.GetHtlcEvents(nil)
	require.NotNil(t, result)

	// Verify it's the same channel
	svc.emitHtlcEvent(HtlcEvent{
		Type:    HtlcEventSpent,
		VhtlcId: "test-channel",
	})

	select {
	case e := <-result:
		require.Equal(t, HtlcEventSpent, e.Type)
		require.Equal(t, "test-channel", e.VhtlcId)
	case <-time.After(time.Second):
		t.Fatal("did not receive event from GetHtlcEvents channel")
	}
}

func TestHtlcEvent_AllTypes(t *testing.T) {
	ch := make(chan HtlcEvent, 1024)
	svc := &Service{htlcEvents: ch}

	tests := []struct {
		name  string
		event HtlcEvent
	}{
		{
			name: "htlc_created",
			event: HtlcEvent{
				Type:      HtlcEventCreated,
				Timestamp: time.Now().Unix(),
				VhtlcId:   "created-id",
				Address:   "tark1...",
			},
		},
		{
			name: "htlc_funded",
			event: HtlcEvent{
				Type:      HtlcEventFunded,
				Timestamp: time.Now().Unix(),
				VhtlcId:   "funded-id",
				TxId:      "abc123",
				Amount:    50000,
			},
		},
		{
			name: "htlc_spent_claimed",
			event: HtlcEvent{
				Type:      HtlcEventSpent,
				Timestamp: time.Now().Unix(),
				VhtlcId:   "claimed-id",
				TxId:      "def456",
				SpendKind: SpendTypeClaimed,
			},
		},
		{
			name: "htlc_spent_refunded",
			event: HtlcEvent{
				Type:      HtlcEventSpent,
				Timestamp: time.Now().Unix(),
				VhtlcId:   "refunded-id",
				TxId:      "ghi789",
				SpendKind: SpendTypeRefunded,
			},
		},
		{
			name: "htlc_refundable",
			event: HtlcEvent{
				Type:           HtlcEventRefundable,
				Timestamp:      time.Now().Unix(),
				VhtlcId:        "refundable-id",
				RefundLocktime: 800000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.emitHtlcEvent(tt.event)

			select {
			case e := <-ch:
				require.Equal(t, tt.event.Type, e.Type)
				require.Equal(t, tt.event.VhtlcId, e.VhtlcId)
				require.Equal(t, tt.event.TxId, e.TxId)
				require.Equal(t, tt.event.Amount, e.Amount)
				require.Equal(t, tt.event.SpendKind, e.SpendKind)
				require.Equal(t, tt.event.RefundLocktime, e.RefundLocktime)
				require.Equal(t, tt.event.Address, e.Address)
			case <-time.After(time.Second):
				t.Fatal("did not receive event")
			}
		})
	}
}
