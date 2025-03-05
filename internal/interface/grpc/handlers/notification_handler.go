package handlers

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notificationHandler struct {
	svc *application.Service

	notificationListenerHandler *listenerHanlder[*pb.GetVtxoNotificationsResponse]
	stopCh                      <-chan struct{}
}

func NewNotificationHandler(
	appSvc *application.Service, stopCh <-chan struct{},
) pb.NotificationServiceServer {
	handler := newListenerHandler[*pb.GetVtxoNotificationsResponse]()
	svc := &notificationHandler{appSvc, handler, stopCh}
	go svc.listenToNotifications()
	return svc
}

func (h *notificationHandler) SubscribeForAddresses(ctx context.Context, req *pb.SubscribeForAddressesRequest) (*pb.SubscribeForAddressesResponse, error) {
	addresses, err := parseAddresses(req.GetAddresses())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := h.svc.SubscribeForAddresses(ctx, addresses); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to subscribe: %s", err))
	}

	return &pb.SubscribeForAddressesResponse{}, nil
}

func (h *notificationHandler) UnsubscribeForAddresses(ctx context.Context, req *pb.UnsubscribeForAddressesRequest) (*pb.UnsubscribeForAddressesResponse, error) {
	addresses, err := parseAddresses(req.GetAddresses())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := h.svc.UnsubscribeForAddresses(ctx, addresses); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to unsubscribe: %s", err))
	}

	return &pb.UnsubscribeForAddressesResponse{}, nil
}

func (h *notificationHandler) GetVtxoNotifications(
	_ *pb.GetVtxoNotificationsRequest, stream pb.NotificationService_GetVtxoNotificationsServer,
) error {
	listener := &listener[*pb.GetVtxoNotificationsResponse]{
		id: uuid.NewString(),
		ch: make(chan *pb.GetVtxoNotificationsResponse),
	}

	h.notificationListenerHandler.pushListener(listener)
	defer h.notificationListenerHandler.removeListener(listener.id)

	for {
		select {
		case <-stream.Context().Done():
			close(listener.ch)
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

func (h *notificationHandler) RoundNotifications(
	req *pb.RoundNotificationsRequest, stream pb.NotificationService_RoundNotificationsServer,
) error {
	return fmt.Errorf("not implemented")
}

func (h *notificationHandler) AddWebhook(
	ctx context.Context, req *pb.AddWebhookRequest,
) (*pb.AddWebhookResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *notificationHandler) RemoveWebhook(
	ctx context.Context, req *pb.RemoveWebhookRequest,
) (*pb.RemoveWebhookResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *notificationHandler) ListWebhooks(
	ctx context.Context, req *pb.ListWebhooksRequest,
) (*pb.ListWebhooksResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *notificationHandler) listenToNotifications() {
	for {
		select {
		case event := <-h.svc.GetVtxoNotifications(context.Background()):
			for _, l := range h.notificationListenerHandler.listeners {
				go func(l *listener[*pb.GetVtxoNotificationsResponse]) {
					l.ch <- &pb.GetVtxoNotificationsResponse{
						Notification: toNotificationProto(event),
					}
				}(l)
			}
		case <-h.stopCh:
			h.notificationListenerHandler.stop()
			return
		}
	}
}

type listener[T any] struct {
	id string
	ch chan T
}

type listenerHanlder[T any] struct {
	lock      *sync.Mutex
	listeners []*listener[T]
}

func newListenerHandler[T any]() *listenerHanlder[T] {
	return &listenerHanlder[T]{
		lock:      &sync.Mutex{},
		listeners: make([]*listener[T], 0),
	}
}

func (h *listenerHanlder[T]) pushListener(l *listener[T]) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.listeners = append(h.listeners, l)
}

func (h *listenerHanlder[T]) removeListener(id string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	for i, listener := range h.listeners {
		if listener.id == id {
			h.listeners = append(h.listeners[:i], h.listeners[i+1:]...)
			return
		}
	}
}

func (h *listenerHanlder[T]) stop() {
	h.lock.Lock()
	defer h.lock.Unlock()
	for _, listener := range h.listeners {
		close(listener.ch)
	}
}
