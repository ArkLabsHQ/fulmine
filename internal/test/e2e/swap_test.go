package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/test/e2e/setup/nigiri"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	swapInvoiceSats    = 3_000
	fulmineGrpcAddress = "localhost:7000"
	operationTimeout   = 10 * time.Minute
	postProcessTimeout = 3 * time.Minute
)

func TestBasicSwap(t *testing.T) {

	t.Run("FulmineInvoice", func(t *testing.T) {
		testFulminePayInvoice(t)
		testFulmineGetInvoice(t)
	})
}

func TestFulmineConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	clients := makeClients(t)

	{
		eg, _ := errgroup.WithContext(ctx)
		for i := range clients {
			i := i
			eg.Go(func() error {
				clients[i].ensureFunding(t)
				return nil
			})
		}
		eg.Wait()
	}

	// TODO: Not really Concurrent yet, needs to be adjusted
	t.Run("ConcurrentRequests", func(t *testing.T) {
		//maxParallel := max(2, runtime.GOMAXPROCS(0)*2)
		maxParallel := 1

		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup

		for i, c := range clients {
			c := c
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()

				if i%2 == 0 {
					testPayInvoice(t, c)
				} else {
					testGetSwapInvoice(t, c)
				}
			}(i)
		}
		wg.Wait()
	})
}

func TestFulmineRefund(t *testing.T) {
	client := newSwapTestClient(t, "tester")

	client.ensureFunding(t)

	t.Run("RefundInvoice", func(t *testing.T) {
		testRefundSwapInvoice(t, client)
	})
}

func testPayInvoice(t *testing.T, c *swapTestClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	invoice, rHash, err := nigiri.AddInvoice(ctx, swapInvoiceSats)
	require.NoError(t, err, "%s: add invoice", c.name)

	swapDetails, err := c.handler.PayInvoice(ctx, invoice, func(s swap.Swap) error {
		return nil
	})
	require.NoError(t, err, "%s: pay invoice via swap handler", c.name)
	require.NotNil(t, swapDetails, "%s: swap response", c.name)
	require.Equal(t, swap.SwapSuccess, swapDetails.Status, "%s: expected successful swap", c.name)
	require.NotEmpty(t, swapDetails.TxId, "%s: missing swap txid", c.name)

	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("%s: context canceled while waiting for invoice settlement: %v", c.name, ctx.Err())
		case <-deadline:
			t.Fatalf("%s: invoice %s not settled before timeout", c.name, rHash)
		case <-ticker.C:
			ok, lookupErr := nigiri.LookupInvoice(ctx, rHash)
			require.NoError(t, lookupErr, "%s: lookup invoice", c.name)
			if ok {
				return
			}
		}
	}

}

func testGetSwapInvoice(t *testing.T, c *swapTestClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	resultCh := make(chan swap.Swap, 1)
	swapDetails, err := c.handler.GetInvoice(ctx, swapInvoiceSats, func(s swap.Swap) error {
		resultCh <- s
		return nil
	})
	require.NoError(t, err, "%s: get swap invoice", c.name)
	require.NotEmpty(t, swapDetails.Invoice, "%s: empty invoice", c.name)

	err = nigiri.PayInvoice(ctx, swapDetails.Invoice)
	require.NoError(t, err, "%s: pay swap invoice", c.name)

	var final swap.Swap
	select {
	case final = <-resultCh:
	case <-time.After(postProcessTimeout):
		t.Fatalf("%s: timeout waiting for swap completion", c.name)
	}

	require.Equal(t, int(swap.SwapSuccess), int(final.Status), "%s: expected successful reverse swap", c.name)
	require.NotEmpty(t, final.RedeemTxid, "%s: missing redeem txid", c.name)
}

func testRefundSwapInvoice(t *testing.T, c *swapTestClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	invoice, rHash, err := nigiri.AddInvoice(ctx, swapInvoiceSats)
	require.NoError(t, err, "%s: add invoice", c.name)

	err = nigiri.CancelInvoice(ctx, rHash)
	require.NoError(t, err, "cancel invoice")

	swapDetails, err := c.handler.PayInvoice(ctx, invoice, func(s swap.Swap) error {
		return nil
	})
	require.NoError(t, err, "%s: pay invoice via swap handler", c.name)
	require.NotNil(t, swapDetails, "%s: swap response", c.name)
	require.Equal(t, swap.SwapFailed, swapDetails.Status, "%s: expected failed swap", c.name)

	require.NotEmpty(t, swapDetails.RedeemTxid, "%s: missing refund txid", c.name)
	require.Equal(t, swapDetails.Status, swap.SwapFailed, "%s: expected failed swap status", c.name)

}

func testFulminePayInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	invoice, rHash, err := nigiri.AddInvoice(ctx, swapInvoiceSats)
	require.NoError(t, err, "add invoice")

	conn, err := grpc.DialContext(
		ctx, fulmineGrpcAddress,
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

func testFulmineGetInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx, fulmineGrpcAddress,
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

	// allow some time for Payment To be processed
	time.Sleep(10 * time.Second)

	// try to settle
	_, err = client.Settle(ctx, &pb.SettleRequest{})
	require.NoError(t, err)

	newBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	require.GreaterOrEqual(t, newBalance.Amount, balance.Amount+2_000)
}

func testFulmineRefundInvoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	invoice, rHash, err := nigiri.AddInvoice(ctx, swapInvoiceSats)
	require.NoError(t, err, "add invoice")

	err = nigiri.CancelInvoice(ctx, rHash)
	require.NoError(t, err, "cancel invoice")

	conn, err := grpc.DialContext(
		ctx, fulmineGrpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)

	require.NoError(t, err, "dial fulmine gRPC")
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	swapResp, err := client.PayInvoice(ctx, &pb.PayInvoiceRequest{Invoice: invoice})
	require.NoError(t, err, "PayInvoice")

	require.NotNil(t, swapResp)
	require.NotEmpty(t, swapResp.Txid)

	newBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	require.Equal(t, newBalance.Amount, balance.Amount)

}

func makeClients(t *testing.T) []*swapTestClient {
	t.Helper()
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}
	var clients []*swapTestClient
	for _, n := range names {
		clients = append(clients, newSwapTestClient(t, n))
	}
	return clients
}
