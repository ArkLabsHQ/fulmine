package interceptors

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// spyMacaroonService captures the context passed to Auth.
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

// fakeServerStream implements grpc.ServerStream with a controllable context.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func TestStreamMacaroonAuth_UsesStreamContext(t *testing.T) {
	spy := &spyMacaroonService{}

	streamCtx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("macaroon", "test-token"),
	)
	ss := &fakeServerStream{ctx: streamCtx}
	info := &grpc.StreamServerInfo{FullMethod: "/fulmine.v1.Service/GetVtxoNotifications"}

	handlerCalled := false
	handler := func(_ interface{}, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := streamMacaroonAuth(spy, nil, ss, info, handler)
	require.NoError(t, err)
	require.True(t, handlerCalled)

	// Critical: Auth must have received the stream context with metadata,
	// NOT an empty context.Background().
	md, ok := metadata.FromIncomingContext(spy.capturedCtx)
	require.True(t, ok, "Auth context must carry incoming gRPC metadata")
	require.Equal(t, []string{"test-token"}, md.Get("macaroon"))
}

func TestStreamMacaroonAuth_ReturnsAuthError(t *testing.T) {
	spy := &spyMacaroonService{errToReturn: fmt.Errorf("permission denied")}

	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/fulmine.v1.Service/GetVtxoNotifications"}

	handlerCalled := false
	handler := func(_ interface{}, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := streamMacaroonAuth(spy, nil, ss, info, handler)
	require.EqualError(t, err, "permission denied")
	require.False(t, handlerCalled, "handler must not be called when auth fails")
}

func TestStreamMacaroonAuth_NilService(t *testing.T) {
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/fulmine.v1.Service/GetVtxoNotifications"}

	handlerCalled := false
	handler := func(_ interface{}, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := streamMacaroonAuth(nil, nil, ss, info, handler)
	require.NoError(t, err)
	require.True(t, handlerCalled, "handler must be called when macaroon service is nil")
}

func TestUnaryMacaroonAuth_UsesRequestContext(t *testing.T) {
	spy := &spyMacaroonService{}

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("macaroon", "unary-token"),
	)
	info := &grpc.UnaryServerInfo{FullMethod: "/fulmine.v1.Service/GetBalance"}

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := unaryMacaroonAuth(spy, ctx, nil, info, handler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	md, ok := metadata.FromIncomingContext(spy.capturedCtx)
	require.True(t, ok, "Auth context must carry incoming gRPC metadata")
	require.Equal(t, []string{"unary-token"}, md.Get("macaroon"))
}
