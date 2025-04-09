package interceptors

import (
	"context"
	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unarySentryErrorReporter reports errors to Sentry
func unarySentryErrorReporter(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		// Only report non-canceled context errors
		if status.Code(err) != codes.Canceled {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("method", info.FullMethod)
				scope.SetContext("request", map[string]interface{}{
					"method": info.FullMethod,
					"data":   req,
				})
				sentry.CaptureException(err)
			})
		}
	}
	return resp, err
}

// streamSentryErrorReporter reports errors to Sentry
func streamSentryErrorReporter(
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	err := handler(srv, stream)
	if err != nil {
		// Only report non-canceled context errors
		if status.Code(err) != codes.Canceled {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("method", info.FullMethod)
				scope.SetContext("stream", map[string]interface{}{
					"method":   info.FullMethod,
					"isClient": info.IsClientStream,
					"isServer": info.IsServerStream,
				})
				sentry.CaptureException(err)
			})
		}
	}
	return err
}
