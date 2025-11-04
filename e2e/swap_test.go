package e2e

import (
	"context"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/e2e/setup/nigiri"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	grpcAddr        = "localhost:7000"
	swapInvoiceSats = 3_000
)

func TestSwap(t *testing.T) {
	t.Run("PayInvoice", func(t *testing.T) {
		testPayInvoice(t)
	})
	t.Run("GetSwapInvoice", func(t *testing.T) {
		testGetSwapInvoice(t)
	})
}

func testPayInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	invoice, rHash, err := nigiri.AddInvoice(ctx, swapInvoiceSats)
	require.NoError(t, err, "add invoice")

	conn, err := grpc.DialContext(
		ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "dial fulmine gRPC")
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	swapResp, err := client.PayInvoice(ctx, &pb.PayInvoiceRequest{Invoice: invoice})
	require.NoError(t, err, "PayInvoice")

	require.NotNil(t, swapResp)
	require.NotEmpty(t, swapResp.Txid)

	deadline := time.Now().Add(2 * time.Minute)
	settled := false
	for time.Now().Before(deadline) {
		ok, lookupErr := nigiri.LookupInvoice(ctx, rHash)
		require.NoError(t, lookupErr, "lookup invoice")

		if ok {
			settled = true
			break
		}
		time.Sleep(2 * time.Second)
	}

	require.True(t, settled, "invoice should settle via nigiri lnd")
}

func testGetSwapInvoice(t *testing.T) {
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
	require.NoError(t, err, "GetInvoice")

	require.NotEmpty(t, resp.GetInvoice(), "invoice")

	err = nigiri.PayInvoice(ctx, resp.GetInvoice())
	require.NoError(t, err, "pay invoice")

	// allow some time for the settlement to be processed
	time.Sleep(1 * time.Minute)

	// try to settle
	_, err = client.Settle(ctx, &pb.SettleRequest{})
	require.NoError(t, err)

	newBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	require.GreaterOrEqual(t, newBalance.Amount, balance.Amount+2_000)
}
