package handlers

import (
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestToEventProto(t *testing.T) {
	now := time.Now()

	t.Run("VHTLC created event", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeVhtlcCreated,
			Timestamp: now,
			Vhtlc: &application.VhtlcEventData{
				ID: "vhtlc-123",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_CREATED, result.Type)
		require.Equal(t, now.Unix(), result.TimestampUnix)
		require.NotNil(t, result.GetVhtlc())
		require.Equal(t, "vhtlc-123", result.GetVhtlc().Id)
		require.Empty(t, result.GetVhtlc().Txid)
		require.Empty(t, result.GetVhtlc().Preimage)
	})

	t.Run("VHTLC funded event", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeVhtlcFunded,
			Timestamp: now,
			Vhtlc: &application.VhtlcEventData{
				ID:   "vhtlc-456",
				Txid: "abc123def456",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_FUNDED, result.Type)
		require.NotNil(t, result.GetVhtlc())
		require.Equal(t, "vhtlc-456", result.GetVhtlc().Id)
		require.Equal(t, "abc123def456", result.GetVhtlc().Txid)
	})

	t.Run("VHTLC claimed event with preimage", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeVhtlcClaimed,
			Timestamp: now,
			Vhtlc: &application.VhtlcEventData{
				ID:       "vhtlc-789",
				Txid:     "txid789",
				Preimage: "deadbeef1234567890",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_CLAIMED, result.Type)
		require.NotNil(t, result.GetVhtlc())
		require.Equal(t, "vhtlc-789", result.GetVhtlc().Id)
		require.Equal(t, "txid789", result.GetVhtlc().Txid)
		require.Equal(t, "deadbeef1234567890", result.GetVhtlc().Preimage)
	})

	t.Run("VHTLC refunded event", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeVhtlcRefunded,
			Timestamp: now,
			Vhtlc: &application.VhtlcEventData{
				ID:   "vhtlc-refund",
				Txid: "refund-txid",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_REFUNDED, result.Type)
		require.NotNil(t, result.GetVhtlc())
		require.Equal(t, "vhtlc-refund", result.GetVhtlc().Id)
		require.Equal(t, "refund-txid", result.GetVhtlc().Txid)
		require.Empty(t, result.GetVhtlc().Preimage)
	})

	t.Run("VTXO received event", func(t *testing.T) {
		vtxos := []types.Vtxo{
			{
				Outpoint: types.Outpoint{Txid: "vtxo-txid-1", VOut: 0},
				Script:   "script1",
				Amount:   1000,
			},
			{
				Outpoint: types.Outpoint{Txid: "vtxo-txid-2", VOut: 1},
				Script:   "script2",
				Amount:   2000,
			},
		}
		event := application.FulmineEvent{
			Type:      application.EventTypeVtxoReceived,
			Timestamp: now,
			Vtxo: &application.VtxoEventData{
				Vtxos: vtxos,
				Txid:  "round-txid",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VTXO_RECEIVED, result.Type)
		require.NotNil(t, result.GetVtxo())
		require.Equal(t, "round-txid", result.GetVtxo().Txid)
		require.Len(t, result.GetVtxo().Vtxos, 2)
		require.Equal(t, "vtxo-txid-1", result.GetVtxo().Vtxos[0].Outpoint.Txid)
		require.Equal(t, uint64(1000), result.GetVtxo().Vtxos[0].Amount)
	})

	t.Run("VTXO spent event", func(t *testing.T) {
		vtxos := []types.Vtxo{
			{
				Outpoint: types.Outpoint{Txid: "spent-txid", VOut: 0},
				Script:   "spent-script",
				Amount:   5000,
				SpentBy:  "spending-tx",
			},
		}
		event := application.FulmineEvent{
			Type:      application.EventTypeVtxoSpent,
			Timestamp: now,
			Vtxo: &application.VtxoEventData{
				Vtxos: vtxos,
				Txid:  "ark-txid",
			},
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VTXO_SPENT, result.Type)
		require.NotNil(t, result.GetVtxo())
		require.Len(t, result.GetVtxo().Vtxos, 1)
		require.Equal(t, "spent-txid", result.GetVtxo().Vtxos[0].Outpoint.Txid)
		require.Equal(t, "spending-tx", result.GetVtxo().Vtxos[0].SpentBy)
	})

	t.Run("unspecified event type", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeUnspecified,
			Timestamp: now,
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_UNSPECIFIED, result.Type)
		require.Nil(t, result.GetVhtlc())
		require.Nil(t, result.GetVtxo())
	})

	t.Run("event with nil data", func(t *testing.T) {
		event := application.FulmineEvent{
			Type:      application.EventTypeVhtlcCreated,
			Timestamp: now,
			Vhtlc:     nil,
			Vtxo:      nil,
		}

		result := toEventProto(event)

		require.Equal(t, pb.EventType_EVENT_TYPE_VHTLC_CREATED, result.Type)
		require.Nil(t, result.GetVhtlc())
		require.Nil(t, result.GetVtxo())
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
