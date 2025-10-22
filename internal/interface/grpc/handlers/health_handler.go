package handlers

import (
	"context"

	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

type healthHandler struct{}

var healthResponse = &grpchealth.HealthCheckResponse{
	Status: grpchealth.HealthCheckResponse_SERVING,
}

func NewHealthHandler() grpchealth.HealthServer {
	return &healthHandler{}
}

func (h *healthHandler) List(context.Context, *grpchealth.HealthListRequest) (*grpchealth.HealthListResponse, error) {
	return &grpchealth.HealthListResponse{
		Statuses: map[string]*grpchealth.HealthCheckResponse{
			"fulmine": healthResponse,
		},
	}, nil
}

func (h *healthHandler) Check(
	_ context.Context,
	_ *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	return healthResponse, nil
}

func (h *healthHandler) Watch(
	_ *grpchealth.HealthCheckRequest,
	_ grpchealth.Health_WatchServer,
) error {
	return nil
}
