package interceptors

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func unaryRecoveryInterceptor(sentryEnabled bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				errValue := fmt.Errorf("panic: %v", r)

				if sentryEnabled {
					sentry.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("method", info.FullMethod)
						scope.SetTag("type", "grpc_unary")
						scope.SetContext("request", map[string]interface{}{
							"method": info.FullMethod,
							"data":   req,
						})
						scope.SetExtra("stack_trace", stackTrace)
						sentry.CaptureException(errValue)
					})

					logFields := log.Fields{
						"method":      info.FullMethod,
						"panic":       r,
						"stack_trace": stackTrace,
						"skip_sentry": true, // Signal that we already sent to Sentry
					}
					log.WithFields(logFields).Error("Panic recovered in gRPC unary interceptor")
				} else {
					log.WithFields(log.Fields{
						"method":      info.FullMethod,
						"panic":       r,
						"stack_trace": stackTrace,
					}).Error("Panic recovered in gRPC unary interceptor")
				}

				err = status.Errorf(codes.Internal, "Internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// streamRecoveryInterceptor recovers from panics in stream handlers and reports them to Sentry
func streamRecoveryInterceptor(sentryEnabled bool) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				errValue := fmt.Errorf("panic: %v", r)

				if sentryEnabled {
					sentry.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("method", info.FullMethod)
						scope.SetTag("type", "grpc_stream")
						scope.SetContext("stream", map[string]interface{}{
							"method":   info.FullMethod,
							"isClient": info.IsClientStream,
							"isServer": info.IsServerStream,
						})
						scope.SetExtra("stack_trace", stackTrace)
						sentry.CaptureException(errValue)
					})

					logFields := log.Fields{
						"method":      info.FullMethod,
						"panic":       r,
						"stack_trace": stackTrace,
						"skip_sentry": true,
					}
					log.WithFields(logFields).Error("Panic recovered in gRPC stream interceptor")
				} else {
					log.WithFields(log.Fields{
						"method":      info.FullMethod,
						"panic":       r,
						"stack_trace": stackTrace,
					}).Error("Panic recovered in gRPC stream interceptor")
				}

				err = status.Errorf(codes.Internal, "Internal server error")
			}
		}()

		return handler(srv, ss)
	}
}
