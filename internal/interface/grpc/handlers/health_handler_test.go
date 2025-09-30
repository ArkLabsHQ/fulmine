package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

// TestHealthCheck_ServiceNotReady tests that health check returns NOT_SERVING when service is not ready
func TestHealthCheck_ServiceNotReady(t *testing.T) {
	// Create a handler with nil service (simulating uninitialized state)
	handler := &healthHandler{svc: nil}

	resp, err := handler.Check(context.Background(), &grpchealth.HealthCheckRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, grpchealth.HealthCheckResponse_NOT_SERVING, resp.Status)
}

// TestHealthCheck_Watch tests the Watch method (stub implementation)
func TestHealthCheck_Watch(t *testing.T) {
	handler := &healthHandler{svc: nil}

	err := handler.Watch(&grpchealth.HealthCheckRequest{}, nil)
	require.NoError(t, err)
}
