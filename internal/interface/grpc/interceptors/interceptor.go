package interceptors

import (
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

// UnaryInterceptor returns the unary interceptor
func UnaryInterceptor(sentryEnabled bool) grpc.ServerOption {
	interceptors := []grpc.UnaryServerInterceptor{
		unaryRecoveryInterceptor(sentryEnabled),
		unaryLogger,
	}
	if sentryEnabled {
		interceptors = append(interceptors, unarySentryErrorReporter)
	}
	return grpc.UnaryInterceptor(middleware.ChainUnaryServer(interceptors...))
}

// StreamInterceptor returns the stream interceptor with a logrus log
func StreamInterceptor(sentryEnabled bool) grpc.ServerOption {
	interceptors := []grpc.StreamServerInterceptor{
		streamRecoveryInterceptor(sentryEnabled),
		streamLogger,
	}
	if sentryEnabled {
		interceptors = append(interceptors, streamSentryErrorReporter)
	}

	return grpc.StreamInterceptor(middleware.ChainStreamServer(interceptors...))
}
