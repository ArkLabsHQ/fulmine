package handlers

import (
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/stretchr/testify/require"
)

func TestToVhtlcEventProto(t *testing.T) {
	now := time.Now()

	t.Run("VHTLC created event", func(t *testing.T) {
		event := application.VhtlcEvent{
			ID:        "vhtlc-123",
			Type:      application.EventTypeVhtlcCreated,
			Timestamp: now,
		}

		result := toVhtlcEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_CREATED, result.Type)
		require.Equal(t, now.Unix(), result.TimestampUnix)
		require.Equal(t, "vhtlc-123", result.Id)
		require.Empty(t, result.Txid)
		require.Empty(t, result.Preimage)
	})

	t.Run("VHTLC funded event", func(t *testing.T) {
		event := application.VhtlcEvent{
			ID:        "vhtlc-456",
			Txid:      "abc123def456",
			Type:      application.EventTypeVhtlcFunded,
			Timestamp: now,
		}

		result := toVhtlcEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_FUNDED, result.Type)
		require.Equal(t, "vhtlc-456", result.Id)
		require.Equal(t, "abc123def456", result.Txid)
	})

	t.Run("VHTLC claimed event with preimage", func(t *testing.T) {
		event := application.VhtlcEvent{
			ID:        "vhtlc-789",
			Txid:      "txid789",
			Preimage:  "deadbeef1234567890",
			Type:      application.EventTypeVhtlcClaimed,
			Timestamp: now,
		}

		result := toVhtlcEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_CLAIMED, result.Type)
		require.Equal(t, "vhtlc-789", result.Id)
		require.Equal(t, "txid789", result.Txid)
		require.Equal(t, "deadbeef1234567890", result.Preimage)
	})

	t.Run("VHTLC refunded event", func(t *testing.T) {
		event := application.VhtlcEvent{
			ID:        "vhtlc-refund",
			Txid:      "refund-txid",
			Type:      application.EventTypeVhtlcRefunded,
			Timestamp: now,
		}

		result := toVhtlcEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_REFUNDED, result.Type)
		require.Equal(t, "vhtlc-refund", result.Id)
		require.Equal(t, "refund-txid", result.Txid)
		require.Empty(t, result.Preimage)
	})

	t.Run("unspecified event type", func(t *testing.T) {
		event := application.VhtlcEvent{
			Type:      application.EventTypeUnspecified,
			Timestamp: now,
		}

		result := toVhtlcEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_UNSPECIFIED, result.Type)
		require.Empty(t, result.Id)
	})
}

func TestListenerHandler(t *testing.T) {
	t.Run("push and remove listeners", func(t *testing.T) {
		handler := newListenerHandler[string]()

		l1 := &listener[string]{id: "listener-1", ch: make(chan string)}
		l2 := &listener[string]{id: "listener-2", ch: make(chan string)}

		handler.pushListener(l1)
		require.Len(t, handler.listeners, 1)

		handler.pushListener(l2)
		require.Len(t, handler.listeners, 2)

		handler.removeListener("listener-1")
		require.Len(t, handler.listeners, 1)
		require.Equal(t, "listener-2", handler.listeners[0].id)

		handler.removeListener("listener-2")
		require.Len(t, handler.listeners, 0)
	})

	t.Run("remove non-existent listener", func(t *testing.T) {
		handler := newListenerHandler[string]()

		l1 := &listener[string]{id: "listener-1", ch: make(chan string)}
		handler.pushListener(l1)

		handler.removeListener("non-existent")
		require.Len(t, handler.listeners, 1)
	})
}
