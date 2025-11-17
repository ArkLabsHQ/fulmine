package e2e_test

import (
	"testing"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

func TestSubmarineSwap(t *testing.T) {
	invoiceAmount := 5000
	client, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, client)

	t.Run("bolt11", func(t *testing.T) {
		invoice, _, err := lndAddInvoice(t.Context(), invoiceAmount)
		require.NoError(t, err)
		require.NotEmpty(t, invoice)

		balance, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balance)
		require.Greater(t, int(balance.GetAmount()), invoiceAmount)

		_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
			Invoice: invoice,
		})
		require.NoError(t, err)

		balanceAfter, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balanceAfter)
		require.GreaterOrEqual(t, int(balance.GetAmount()-balanceAfter.GetAmount()), invoiceAmount)
	})

	t.Run("bolt12", func(t *testing.T) {
		invoice, _, err := clnAddOffer(t.Context(), invoiceAmount*1000)
		require.NoError(t, err)
		require.NotEmpty(t, invoice)

		balance, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balance)
		require.Greater(t, int(balance.GetAmount()), invoiceAmount)

		_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
			Invoice: invoice,
		})
		require.NoError(t, err)

		balanceAfter, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balanceAfter)
		require.GreaterOrEqual(t, int(balance.GetAmount()-balanceAfter.GetAmount()), invoiceAmount)
	})

	// TODO: uncomment this in PR#330
	// t.Run("refund", func(t *testing.T) {
	// 	invoice, rHash, err := lndAddInvoice(t.Context(), 5000)
	// 	require.NoError(t, err)
	// 	require.NotEmpty(t, invoice)
	// 	require.NotEmpty(t, rHash)

	// 	err = lndCancelInvoice(t.Context(), rHash)
	// 	require.NoError(t, err)

	// 	balance, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
	// 	require.NoError(t, err)
	// 	require.NotNil(t, balance)
	// 	require.Greater(t, int(balance.GetAmount()), 5000)

	// 	_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
	// 		Invoice: invoice,
	// 	})
	// 	require.NoError(t, err)

	// 	balanceAfter, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
	// 	require.NoError(t, err)
	// 	require.NotNil(t, balanceAfter)
	// 	require.Zero(t, int(balance.GetAmount()-balanceAfter.GetAmount()))
	// })
}

func TestReverseSwap(t *testing.T) {
	invoiceAmount := 3000
	client, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, client)

	t.Run("valid", func(t *testing.T) {
		balance, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balance)
		require.Greater(t, int(balance.GetAmount()), invoiceAmount)

		invoice, err := client.GetInvoice(t.Context(), &pb.GetInvoiceRequest{
			Amount: uint64(invoiceAmount),
		})
		require.NoError(t, err)
		require.NotNil(t, invoice)
		require.NotEmpty(t, invoice.GetInvoice())

		err = lndPayInvoice(t.Context(), invoice.GetInvoice())
		require.NoError(t, err)

		balanceAfter, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balanceAfter)
		require.LessOrEqual(t, int(balanceAfter.GetAmount()-balance.GetAmount()), invoiceAmount)
	})
}

// TOOD: add tests for increase inbound/outbound
