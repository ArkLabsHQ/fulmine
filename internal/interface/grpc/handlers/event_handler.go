package handlers

import (
	"context"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// GetEventStream implements the gRPC server-side streaming RPC.
// Each connected client gets a dedicated listener channel; events are
// fanned-out from the background goroutine started in NewServiceHandler.
func (h *serviceHandler) GetEventStream(
	_ *pb.GetEventStreamRequest, stream pb.Service_GetEventStreamServer,
) error {
	listener := &listener[*pb.GetEventStreamResponse]{
		id: uuid.NewString(),
		ch: make(chan *pb.GetEventStreamResponse),
	}

	h.eventListenerHandler.pushListener(listener)
	defer h.eventListenerHandler.removeListener(listener.id)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case ev, ok := <-listener.ch:
			if !ok {
				return nil
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}

// listenToHtlcEvents reads HTLC lifecycle events from the application service
// and fans them out to all connected event stream listeners.
func (h *serviceHandler) listenToHtlcEvents() {
	htlcCh := h.svc.GetHtlcEvents(context.Background())
	for {
		select {
		case event, ok := <-htlcCh:
			if !ok {
				return
			}
			pbEvent := toEventStreamResponse(event)
			h.eventListenerHandler.lock.Lock()
			for _, l := range h.eventListenerHandler.listeners {
				go func(l *listener[*pb.GetEventStreamResponse]) {
					l.ch <- pbEvent
				}(l)
			}
			h.eventListenerHandler.lock.Unlock()
		case <-h.stopCh:
			h.eventListenerHandler.stop()
			return
		}
	}
}

// toEventStreamResponse converts an application-layer HtlcEvent into the
// protobuf GetEventStreamResponse with the appropriate oneof variant.
func toEventStreamResponse(e application.HtlcEvent) *pb.GetEventStreamResponse {
	resp := &pb.GetEventStreamResponse{Timestamp: e.Timestamp}
	switch e.Type {
	case application.HtlcEventCreated:
		resp.Event = &pb.GetEventStreamResponse_HtlcCreated{
			HtlcCreated: &pb.HtlcCreatedEvent{
				VhtlcId: e.VhtlcId,
				Address: e.Address,
				Amount:  e.Amount,
			},
		}
	case application.HtlcEventFunded:
		resp.Event = &pb.GetEventStreamResponse_HtlcFunded{
			HtlcFunded: &pb.HtlcFundedEvent{
				VhtlcId:     e.VhtlcId,
				FundingTxid: e.TxId,
				Amount:      e.Amount,
			},
		}
	case application.HtlcEventSpent:
		spendType := pb.HtlcSpentEvent_SPEND_TYPE_UNSPECIFIED
		switch e.SpendKind {
		case application.SpendTypeClaimed:
			spendType = pb.HtlcSpentEvent_SPEND_TYPE_CLAIMED
		case application.SpendTypeRefunded:
			spendType = pb.HtlcSpentEvent_SPEND_TYPE_REFUNDED
		}
		resp.Event = &pb.GetEventStreamResponse_HtlcSpent{
			HtlcSpent: &pb.HtlcSpentEvent{
				VhtlcId:    e.VhtlcId,
				RedeemTxid: e.TxId,
				SpendType:  spendType,
			},
		}
	case application.HtlcEventRefundable:
		resp.Event = &pb.GetEventStreamResponse_HtlcRefundable{
			HtlcRefundable: &pb.HtlcRefundableEvent{
				VhtlcId:        e.VhtlcId,
				RefundLocktime: e.RefundLocktime,
			},
		}
	default:
		log.Warnf("unknown htlc event type: %s", e.Type)
	}
	return resp
}

// listenToChainSwapEvents reads chain swap lifecycle events from the application
// service and fans them out to all connected event stream listeners.
func (h *serviceHandler) listenToChainSwapEvents() {
	chainSwapCh := h.svc.GetChainSwapEvents(context.Background())
	for {
		select {
		case event, ok := <-chainSwapCh:
			if !ok {
				return
			}
			pbEvent := toChainSwapEventStreamResponse(event)
			h.eventListenerHandler.lock.Lock()
			for _, l := range h.eventListenerHandler.listeners {
				go func(l *listener[*pb.GetEventStreamResponse]) {
					l.ch <- pbEvent
				}(l)
			}
			h.eventListenerHandler.lock.Unlock()
		case <-h.stopCh:
			// listenToHtlcEvents owns the eventListenerHandler.stop() call.
			// This goroutine just exits.
			return
		}
	}
}

// toChainSwapEventStreamResponse converts a ChainSwapEvent into the protobuf
// GetEventStreamResponse with the appropriate oneof variant.
func toChainSwapEventStreamResponse(e application.ChainSwapEvent) *pb.GetEventStreamResponse {
	resp := &pb.GetEventStreamResponse{Timestamp: e.Timestamp}
	switch e.Type {
	case application.ChainSwapEventCreated:
		resp.Event = &pb.GetEventStreamResponse_ChainSwapCreated{
			ChainSwapCreated: &pb.ChainSwapCreatedEvent{
				SwapId:    e.SwapId,
				Direction: e.Direction,
				Amount:    e.Amount,
			},
		}
	case application.ChainSwapEventUserLocked:
		resp.Event = &pb.GetEventStreamResponse_ChainSwapUserLocked{
			ChainSwapUserLocked: &pb.ChainSwapUserLockedEvent{
				SwapId:         e.SwapId,
				UserLockupTxid: e.UserLockTxId,
			},
		}
	case application.ChainSwapEventServerLocked:
		resp.Event = &pb.GetEventStreamResponse_ChainSwapServerLocked{
			ChainSwapServerLocked: &pb.ChainSwapServerLockedEvent{
				SwapId:           e.SwapId,
				ServerLockupTxid: e.ServerLockTxId,
			},
		}
	case application.ChainSwapEventClaimed:
		resp.Event = &pb.GetEventStreamResponse_ChainSwapClaimed{
			ChainSwapClaimed: &pb.ChainSwapClaimedEvent{
				SwapId:    e.SwapId,
				ClaimTxid: e.ClaimTxId,
			},
		}
	case application.ChainSwapEventRefunded:
		kind := pb.ChainSwapRefundedEvent_REFUND_KIND_UNSPECIFIED
		switch e.RefundKind {
		case application.ChainSwapRefundCooperative:
			kind = pb.ChainSwapRefundedEvent_REFUND_KIND_COOPERATIVE
		case application.ChainSwapRefundUnilateral:
			kind = pb.ChainSwapRefundedEvent_REFUND_KIND_UNILATERAL
		}
		resp.Event = &pb.GetEventStreamResponse_ChainSwapRefunded{
			ChainSwapRefunded: &pb.ChainSwapRefundedEvent{
				SwapId:     e.SwapId,
				RefundTxid: e.RefundTxId,
				Kind:       kind,
			},
		}
	case application.ChainSwapEventFailed:
		resp.Event = &pb.GetEventStreamResponse_ChainSwapFailed{
			ChainSwapFailed: &pb.ChainSwapFailedEvent{
				SwapId:       e.SwapId,
				ErrorMessage: e.ErrorMessage,
			},
		}
	default:
		log.Warnf("unknown chain swap event type: %s", e.Type)
	}
	return resp
}

// toTxAssociatedResponse converts a VTXO notification into a
// GetEventStreamResponse with TxAssociatedEvent.
func toTxAssociatedResponse(n application.Notification) *pb.GetEventStreamResponse {
	return &pb.GetEventStreamResponse{
		Timestamp: time.Now().Unix(),
		Event: &pb.GetEventStreamResponse_TxAssociated{
			TxAssociated: &pb.TxAssociatedEvent{
				Txid:       n.Txid,
				Tx:         n.Tx,
				NewVtxos:   toVtxosProto(n.NewVtxos),
				SpentVtxos: toVtxosProto(n.SpentVtxos),
			},
		},
	}
}
