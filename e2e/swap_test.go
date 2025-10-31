package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/e2e/setup/lightning"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	grpcAddr        = "localhost:7000"
	swapInvoiceSats = 3_000
)

func TestPayInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	invoice, rHash, err := lightning.NigiriAddInvoice(ctx, 3000)
	if err != nil {
		if shouldSkipCommand(err) {
			t.Skipf("nigiri not available: %v", err)
		}
		require.NoError(t, err, "add invoice")
	}

	conn, err := grpc.DialContext(
		ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "dial fulmine gRPC")
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	time.Sleep(5 * time.Second)

	swapResp, err := client.PayInvoice(ctx, &pb.PayInvoiceRequest{Invoice: invoice})
	if err != nil {
		if shouldSkipSwap(err) {
			t.Skipf("swap infrastructure not ready: %v", err)
		}
		require.NoError(t, err, "PayInvoice")
	}

	require.NotNil(t, swapResp)
	require.NotEmpty(t, swapResp.Txid)

	deadline := time.Now().Add(2 * time.Minute)
	settled := false
	for time.Now().Before(deadline) {
		ok, lookupErr := lightning.NigiriLookupInvoice(ctx, rHash)
		if lookupErr != nil {
			if shouldSkipCommand(lookupErr) {
				t.Skipf("nigiri not available: %v", lookupErr)
			}
			t.Logf("waiting for invoice settlement: %v", lookupErr)
		}
		if ok {
			settled = true
			break
		}
		time.Sleep(2 * time.Second)
	}

	require.True(t, settled, "invoice should settle via nigiri lnd")
}

func TestReverseSwapInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "dial fulmine gRPC")
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	resp, err := client.GetInvoice(ctx, &pb.GetInvoiceRequest{Amount: swapInvoiceSats})
	if err != nil {
		if shouldSkipSwap(err) {
			t.Skipf("swap infrastructure not ready: %v", err)
		}
		require.NoError(t, err, "GetInvoice")

	}

	require.NotEmpty(t, resp.GetInvoice(), "invoice")

	err = lightning.NigiriPayInvoice(ctx, resp.GetInvoice())
	if err != nil {
		if shouldSkipCommand(err) {
			t.Skipf("nigiri not available: %v", err)
		}
		require.NoError(t, err, "pay reverse swap invoice")
	}

	// allow some time for the settlement to be processed
	time.Sleep(1 * time.Minute)

	// try to settle
	_, err = client.Settle(ctx, &pb.SettleRequest{})
	require.NoError(t, err)

	newBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	require.GreaterOrEqual(t, newBalance.Amount, balance.Amount+2_000)
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

func shouldSkipCommand(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") && strings.Contains(msg, "docker")
}
