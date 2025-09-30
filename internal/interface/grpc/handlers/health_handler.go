package handlers

import (
	"context"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	log "github.com/sirupsen/logrus"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

type healthHandler struct {
	svc *application.Service
}

func NewHealthHandler(svc *application.Service) grpchealth.HealthServer {
	return &healthHandler{svc: svc}
}

func (h *healthHandler) Check(
	ctx context.Context,
	_ *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	// Check if the service exists and is ready (wallet initialized)
	if h.svc == nil || !h.svc.IsReady() {
		log.Debug("health check: service not ready (wallet not initialized)")
		return &grpchealth.HealthCheckResponse{
			Status: grpchealth.HealthCheckResponse_NOT_SERVING,
		}, nil
	}

	// Check arkd connectivity by attempting to get config data with a short timeout
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := h.svc.GetConfigData(checkCtx)
	if err != nil {
		log.WithError(err).Warn("health check: failed to connect to arkd")
		return &grpchealth.HealthCheckResponse{
			Status: grpchealth.HealthCheckResponse_NOT_SERVING,
		}, nil
	}

	return &grpchealth.HealthCheckResponse{
		Status: grpchealth.HealthCheckResponse_SERVING,
	}, nil
}

func (h *healthHandler) Watch(
	_ *grpchealth.HealthCheckRequest,
	_ grpchealth.Health_WatchServer,
) error {
	return nil
}
