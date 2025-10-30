package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	grpcAddr        = "localhost:7000"
	swapInvoiceSats = 10_000
)

func TestReverseSwapInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "dial fulmine gRPC")
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	resp, err := client.GetInvoice(ctx, &pb.GetInvoiceRequest{Amount: swapInvoiceSats})
	if err != nil {
		if shouldSkipSwap(err) {
			t.Skipf("swap infrastructure not ready: %v", err)
		}
		require.NoError(t, err, "GetInvoice")
	}

	require.NotEmpty(t, resp.GetInvoice(), "invoice")

	statusResp, err := client.IsInvoiceSettled(ctx, &pb.IsInvoiceSettledRequest{
		Invoice: resp.GetInvoice(),
	})
	if err != nil {
		if shouldSkipSwap(err) {
			t.Skipf("swap infrastructure not ready for settlement status: %v", err)
		}
		require.NoError(t, err, "IsInvoiceSettled")
	}

	require.False(t, statusResp.GetSettled(), "invoice should be pending immediately after creation")
}

func shouldSkipSwap(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	if st.Message() == "" {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(strings.ToLower(st.Message()), "boltz") ||
		strings.Contains(strings.ToLower(st.Message()), "connection refused") ||
		strings.Contains(strings.ToLower(st.Message()), "timeout")
}
