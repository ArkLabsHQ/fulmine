package handlers

import (
	"context"
	"fmt"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
)

type notificationHandler struct{}

func NewNotificationHandler() pb.NotificationServiceServer {
	return &notificationHandler{}
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
