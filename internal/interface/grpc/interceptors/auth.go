package interceptors

import (
	"context"
	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"google.golang.org/grpc"
)

func MacaroonAuthInterceptor(macaroonSvc macaroon.Service) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(
		func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			if macaroonSvc != nil {
				if err := macaroonSvc.Auth(ctx, info.FullMethod); err != nil {
					return nil, err
				}
			}
			return handler(ctx, req)
		},
	)
}

func MacaroonStreamAuthInterceptor(macaroonSvc macaroon.Service) grpc.ServerOption {
	return grpc.ChainStreamInterceptor(
		func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			if macaroonSvc != nil {
				if err := macaroonSvc.Auth(context.Background(), info.FullMethod); err != nil {
					return err
				}
			}
			return handler(srv, ss)
		},
	)
}
