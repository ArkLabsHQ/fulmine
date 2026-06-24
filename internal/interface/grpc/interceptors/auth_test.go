package interceptors

import (
	"context"
	"fmt"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestStreamMacaroonAuth(t *testing.T) {
	tests := []struct {
		name           string
		macaroonSvc    macaroon.Service
		streamCtx      context.Context
		expectErr      string
		expectHandler  bool
		expectMacaroon string
	}{
		{
			name:           "valid macaroon",
			macaroonSvc:    &spyMacaroonService{},
			streamCtx:      metadata.NewIncomingContext(context.Background(), metadata.Pairs("macaroon", "test-token")),
			expectHandler:  true,
			expectMacaroon: "test-token",
		},
		{
			name:          "invalid macaroon",
			macaroonSvc:   &spyMacaroonService{errToReturn: fmt.Errorf("permission denied")},
			streamCtx:     context.Background(),
			expectErr:     "permission denied",
			expectHandler: false,
		},
		{
			name:          "nil service",
			macaroonSvc:   nil,
			streamCtx:     context.Background(),
			expectHandler: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := &fakeServerStream{ctx: tt.streamCtx}
			info := &grpc.StreamServerInfo{FullMethod: "/fulmine.v1.NotificationService/GetVtxoNotifications"}

			handlerCalled := false
			handler := func(_ interface{}, _ grpc.ServerStream) error {
				handlerCalled = true
				return nil
			}

			err := streamMacaroonAuth(tt.macaroonSvc, nil, ss, info, handler)

			if tt.expectErr != "" {
				require.EqualError(t, err, tt.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectHandler, handlerCalled)

			if tt.expectMacaroon != "" {
				spy := tt.macaroonSvc.(*spyMacaroonService)
				md, ok := metadata.FromIncomingContext(spy.capturedCtx)
				require.True(t, ok, "Auth context must carry incoming gRPC metadata")
				require.Equal(t, []string{tt.expectMacaroon}, md.Get("macaroon"))
			}
		})
	}
}

func TestUnaryMacaroonAuth(t *testing.T) {
	tests := []struct {
		name           string
		macaroonSvc    macaroon.Service
		ctx            context.Context
		expectErr      string
		expectHandler  bool
		expectMacaroon string
	}{
		{
			name:           "valid macaroon",
			macaroonSvc:    &spyMacaroonService{},
			ctx:            metadata.NewIncomingContext(context.Background(), metadata.Pairs("macaroon", "unary-token")),
			expectHandler:  true,
			expectMacaroon: "unary-token",
		},
		{
			name:          "invalid macaroon",
			macaroonSvc:   &spyMacaroonService{errToReturn: fmt.Errorf("permission denied")},
			ctx:           context.Background(),
			expectErr:     "permission denied",
			expectHandler: false,
		},
		{
			name:          "nil service",
			macaroonSvc:   nil,
			ctx:           context.Background(),
			expectHandler: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &grpc.UnaryServerInfo{FullMethod: "/fulmine.v1.Service/GetBalance"}

			handlerCalled := false
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				handlerCalled = true
				return "ok", nil
			}

			_, err := unaryMacaroonAuth(tt.macaroonSvc, tt.ctx, nil, info, handler)

			if tt.expectErr != "" {
				require.EqualError(t, err, tt.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectHandler, handlerCalled)

			if tt.expectMacaroon != "" {
				spy := tt.macaroonSvc.(*spyMacaroonService)
				md, ok := metadata.FromIncomingContext(spy.capturedCtx)
				require.True(t, ok, "Auth context must carry incoming gRPC metadata")
				require.Equal(t, []string{tt.expectMacaroon}, md.Get("macaroon"))
			}
		})
	}
}

// Test helpers

type spyMacaroonService struct {
	capturedCtx context.Context
	errToReturn error
}

func (s *spyMacaroonService) Auth(ctx context.Context, _ string) error {
	s.capturedCtx = ctx
	return s.errToReturn
}

func (s *spyMacaroonService) Unlock(context.Context, string) error                { return nil }
func (s *spyMacaroonService) ChangePassword(context.Context, string, string) error { return nil }
func (s *spyMacaroonService) Generate(context.Context) error                       { return nil }
func (s *spyMacaroonService) Reset(context.Context) error                          { return nil }

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }
