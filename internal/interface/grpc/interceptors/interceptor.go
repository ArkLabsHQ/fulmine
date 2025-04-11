package interceptors

import (
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_sentry "github.com/johnbellone/grpc-middleware-sentry"
	"google.golang.org/grpc"
)

// UnaryInterceptor returns the unary interceptor chain
func UnaryInterceptor(sentryEnabled bool) grpc.ServerOption {
	interceptors := []grpc.UnaryServerInterceptor{unaryLogger}
	if sentryEnabled {
		sentryOpts := []grpc_sentry.Option{
			grpc_sentry.WithRepanicOption(false),   // Don't re-panic after recovery
			grpc_sentry.WithWaitForDelivery(false), // Don't wait for Sentry to deliver
		}
		interceptors = append([]grpc.UnaryServerInterceptor{
			grpc_sentry.UnaryServerInterceptor(sentryOpts...),
		}, interceptors...)
	}

	return grpc.UnaryInterceptor(middleware.ChainUnaryServer(interceptors...))
}

// StreamInterceptor returns the stream interceptor chain
func StreamInterceptor(sentryEnabled bool) grpc.ServerOption {
	interceptors := []grpc.StreamServerInterceptor{streamLogger}
	if sentryEnabled {
		sentryOpts := []grpc_sentry.Option{
			grpc_sentry.WithRepanicOption(false),   // Don't re-panic after recovery
			grpc_sentry.WithWaitForDelivery(false), // Don't wait for Sentry to deliver
		}
		interceptors = append([]grpc.StreamServerInterceptor{
			grpc_sentry.StreamServerInterceptor(sentryOpts...),
		}, interceptors...)
	}

	return grpc.StreamInterceptor(middleware.ChainStreamServer(interceptors...))
}
