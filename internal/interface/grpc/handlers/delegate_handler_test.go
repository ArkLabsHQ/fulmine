package handlers

import (
	"context"
	"testing"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

func TestFlushDelegateQueueValidates(t *testing.T) {
	// With a nil svc, a request must not panic on the trivial flush path guard.
	// (Full behavior is covered by the application-layer tests; this asserts the
	// handler wiring/response type.)
	h := &delegateHandler{}
	_, err := h.FlushDelegateQueue(context.Background(), &pb.FlushDelegateQueueRequest{})
	require.Error(t, err) // svc nil -> error, not panic
}
