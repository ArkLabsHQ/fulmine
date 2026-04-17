package interceptors

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"google.golang.org/grpc"
)

func MacaroonAuthInterceptor(macaroonSvc macaroon.Service) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(
		func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return unaryMacaroonAuth(macaroonSvc, ctx, req, info, handler)
		},
	)
}

func MacaroonStreamAuthInterceptor(macaroonSvc macaroon.Service) grpc.ServerOption {
	return grpc.ChainStreamInterceptor(
		func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return streamMacaroonAuth(macaroonSvc, srv, ss, info, handler)
		},
	)
}

func unaryMacaroonAuth(
	macaroonSvc macaroon.Service,
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	if macaroonSvc != nil {
		if err := macaroonSvc.Auth(ctx, info.FullMethod); err != nil {
			return nil, err
		}
	}
	return handler(ctx, req)
}

func streamMacaroonAuth(
	macaroonSvc macaroon.Service,
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	if macaroonSvc != nil {
		if err := macaroonSvc.Auth(ss.Context(), info.FullMethod); err != nil {
			return err
		}
	}
	return handler(srv, ss)
}
