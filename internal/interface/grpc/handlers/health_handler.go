package handlers

import (
	"context"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const (
	serviceFulmine = "fulmine"
	serviceLN      = "ln"
)

// https://github.com/grpc/grpc/blob/master/doc/health-checking.md
type healthHandler struct {
	svc *application.Service
}

func NewHealthHandler(svc *application.Service) grpchealth.HealthServer {
	return &healthHandler{svc}
}

func (h *healthHandler) List(
	ctx context.Context, _ *grpchealth.HealthListRequest,
) (*grpchealth.HealthListResponse, error) {
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
	ctx context.Context, req *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	serviceName := req.GetService()
	if err := validateServiceName(serviceName); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return h.getServiceStatus(ctx, serviceName), nil
}

func (h *healthHandler) Watch(
	req *grpchealth.HealthCheckRequest, stream grpchealth.Health_WatchServer,
) error {
	return status.Error(codes.Unimplemented, "method Watch is not implemented")
}

func (h *healthHandler) getServiceStatus(
	ctx context.Context, serviceName string,
) *grpchealth.HealthCheckResponse {
	status := &grpchealth.HealthCheckResponse{
		Status: grpchealth.HealthCheckResponse_UNKNOWN,
	}
	switch serviceName {
	case serviceFulmine:
		status.Status = h.getFulmineStatus(ctx)
	case serviceLN:
		status.Status = h.getLNStatus()
	}
	return status
}

func (h *healthHandler) getFulmineStatus(ctx context.Context) grpchealth.HealthCheckResponse_ServingStatus {
	isSynced, err := h.svc.IsSynced()
	if err != nil {
		log.WithError(err).Warn("failed to get synced status, sending UNKNOWN")
		return grpchealth.HealthCheckResponse_NOT_SERVING
	}
	isInitialized := h.svc.IsInitialized()
	isLocked := h.svc.IsLocked(ctx)

	if isSynced && isInitialized && !isLocked {
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
	case "":
		return fmt.Errorf("missing service name")
	default:
		return fmt.Errorf("invalid service name: %s", requested)
	}
}
