package interceptors

import (
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpc_sentry "github.com/johnbellone/grpc-middleware-sentry"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"runtime/debug"
)

var (
	grpcPanicRecoveryHandler = func(p any) (err error) {
		log.Errorf("panic-recovery middleware recovered from panic: %v", p)
		log.Tracef("panic-recovery middleware recovered from panic: %v", string(debug.Stack()))
		return status.Errorf(codes.Internal, "%s", p)
	}
)

// UnaryInterceptor returns the unary interceptor chain
func UnaryInterceptor(sentryEnabled bool) grpc.ServerOption {
	panicInterceptor := recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler))
	interceptors := []grpc.UnaryServerInterceptor{unaryLogger, panicInterceptor}
	if sentryEnabled {
		sentryOpts := []grpc_sentry.Option{
			grpc_sentry.WithRepanicOption(true),
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
	panicInterceptor := recovery.StreamServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler))
	interceptors := []grpc.StreamServerInterceptor{streamLogger, panicInterceptor}
	if sentryEnabled {
		sentryOpts := []grpc_sentry.Option{
			grpc_sentry.WithRepanicOption(true),
			grpc_sentry.WithWaitForDelivery(false), // Don't wait for Sentry to deliver
		}
		interceptors = append([]grpc.StreamServerInterceptor{
			grpc_sentry.StreamServerInterceptor(sentryOpts...),
		}, interceptors...)
	}

	return grpc.StreamInterceptor(middleware.ChainStreamServer(interceptors...))
}
