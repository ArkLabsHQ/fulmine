package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const (
	serviceFulmine = "fulmine"
	serviceLN      = "ln"
	watchInterval  = time.Minute
)

type healthHandler struct {
	svc *application.Service
}

func NewHealthHandler(svc *application.Service) grpchealth.HealthServer {
	return &healthHandler{svc}
}

func (h *healthHandler) List(ctx context.Context, _ *grpchealth.HealthListRequest) (*grpchealth.HealthListResponse, error) {
	statuses := make(map[string]*grpchealth.HealthCheckResponse)
	statuses[serviceFulmine] = &grpchealth.HealthCheckResponse{
		Status: h.getFulmineStatus(ctx),
	}
	statuses[serviceLN] = &grpchealth.HealthCheckResponse{
		Status: h.getLNStatus(),
	}

	return &grpchealth.HealthListResponse{
		Statuses: statuses,
	}, nil
}

func (h *healthHandler) Check(
	ctx context.Context,
	req *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	serviceName := req.GetService()
	if err := validateServiceName(serviceName); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	switch serviceName {
	case serviceFulmine:
		return &grpchealth.HealthCheckResponse{
			Status: h.getFulmineStatus(ctx),
		}, nil
	case serviceLN:
		return &grpchealth.HealthCheckResponse{
			Status: h.getLNStatus(),
		}, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown service: %s", serviceName)
	}
}

func (h *healthHandler) Watch(
	req *grpchealth.HealthCheckRequest,
	stream grpchealth.Health_WatchServer,
) error {
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()

	serviceName := req.GetService()
	if err := validateServiceName(serviceName); err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	go func() {
		for {
			select {
			case <-stream.Context().Done():
				return
			case <-ticker.C:
				switch serviceName {
				case serviceFulmine:
					if err := stream.Send(&grpchealth.HealthCheckResponse{
						Status: h.getFulmineStatus(stream.Context()),
					}); err != nil {
						log.Errorf("failed to send health check response: %v", err)
						return
					}
				case serviceLN:
					if err := stream.Send(&grpchealth.HealthCheckResponse{
						Status: h.getLNStatus(),
					}); err != nil {
						log.Errorf("failed to send health check response: %v", err)
						return
					}
				}
			}
		}
	}()

	return nil
}

func (h *healthHandler) getFulmineStatus(ctx context.Context) grpchealth.HealthCheckResponse_ServingStatus {
	isSynced, err := h.svc.IsSynced()
	if err != nil {
		log.WithError(err).Warn("failed to get synced status, sending NOT_SERVING")
		return grpchealth.HealthCheckResponse_NOT_SERVING
	}
	isInitialized := h.svc.IsInitialized()
	isLocked := h.svc.IsLocked(ctx)

	if isSynced && isInitialized && isLocked {
		return grpchealth.HealthCheckResponse_SERVING
	}
	return grpchealth.HealthCheckResponse_NOT_SERVING
}

func (h *healthHandler) getLNStatus() grpchealth.HealthCheckResponse_ServingStatus {
	if h.svc.IsConnectedLN() {
		return grpchealth.HealthCheckResponse_SERVING
	}
	return grpchealth.HealthCheckResponse_NOT_SERVING
}

func validateServiceName(requested string) error {
	switch requested {
	case serviceFulmine, serviceLN:
		return nil
	default:
		return fmt.Errorf("invalid service name: %s", requested)
	}
}
