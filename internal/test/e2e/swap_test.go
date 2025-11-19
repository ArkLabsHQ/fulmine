package e2e_test

import (
	"fmt"
	"sync"
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
		before := int64(balance.GetAmount())
		after := int64(balanceAfter.GetAmount())
		require.GreaterOrEqual(t, before-after, int64(invoiceAmount))
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
		before := int64(balance.GetAmount())
		after := int64(balanceAfter.GetAmount())
		require.GreaterOrEqual(t, before-after, int64(invoiceAmount))
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
	// 	before := int64(balance.GetAmount())
	// 	after := int64(balanceAfter.GetAmount())
	// 	require.Zero(t, before-after)
	// })
}

func TestReverseSwap(t *testing.T) {
	invoiceAmount := 4000
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
		before := int64(balance.GetAmount())
		after := int64(balanceAfter.GetAmount())
		require.LessOrEqual(t, after-before, int64(invoiceAmount))
	})
}

func TestCircularSwap(t *testing.T) {
	invoiceAmount := 3000
	client, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, client)

	invoice, err := client.GetInvoice(t.Context(), &pb.GetInvoiceRequest{
		Amount: uint64(invoiceAmount),
	})
	require.NoError(t, err)
	require.NotNil(t, invoice)
	require.NotEmpty(t, invoice.GetInvoice())

	resp, err := client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
		Invoice: invoice.GetInvoice(),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.GetTxid())
}

func TestConcurrentSwaps(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Run("distinct submarine swaps", func(t *testing.T) {
			invoiceAmount := 2000
			invoice1, _, err := lndAddInvoice(t.Context(), invoiceAmount)
			require.NoError(t, err)
			require.NotEmpty(t, invoice1)
			invoice2, _, err := lndAddInvoice(t.Context(), invoiceAmount)
			require.NoError(t, err)
			require.NotEmpty(t, invoice2)

			wg := &sync.WaitGroup{}
			wg.Add(2)

			errs := errs{
				mu:   &sync.Mutex{},
				errs: make([]error, 0, 2),
			}
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
					Invoice: invoice1,
				})
				errs.add(err)
			}()
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
					Invoice: invoice2,
				})
				errs.add(err)
			}()
			wg.Wait()

			require.Len(t, errs.errs, 2)
			require.NoError(t, errs.errs[0])
			require.NoError(t, errs.errs[1])
		})
		t.Run("submarine and reverse swaps", func(t *testing.T) {
			invoiceAmount := 2001
			invoice, _, err := lndAddInvoice(t.Context(), invoiceAmount)
			require.NoError(t, err)
			require.NotEmpty(t, invoice)

			wg := &sync.WaitGroup{}
			wg.Add(2)

			errs := errs{
				mu:   &sync.Mutex{},
				errs: make([]error, 0, 2),
			}
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
					Invoice: invoice,
				})
				errs.add(err)
			}()
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				invoice, err := client.GetInvoice(t.Context(), &pb.GetInvoiceRequest{
					Amount: uint64(invoiceAmount),
				})
				if err != nil {
					errs.add(err)
					return
				}
				err = lndPayInvoice(t.Context(), invoice.GetInvoice())
				errs.add(err)
			}()
			wg.Wait()

			require.Len(t, errs.errs, 2)
			errCount := 0
			for _, err := range errs.errs {
				if err != nil {
					fmt.Println("AAAA", err)
					errCount++
				}
			}
			require.Zero(t, errCount)
		})
		t.Run("distinct reverse swaps", func(t *testing.T) {
			invoiceAmount := uint64(2002)

			wg := &sync.WaitGroup{}
			wg.Add(2)

			errs := errs{
				mu:   &sync.Mutex{},
				errs: make([]error, 0, 2),
			}
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				invoice, err := client.GetInvoice(t.Context(), &pb.GetInvoiceRequest{
					Amount: invoiceAmount,
				})
				if err != nil {
					errs.add(err)
					return
				}
				err = lndPayInvoice(t.Context(), invoice.GetInvoice())
				errs.add(err)
			}()
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				invoice, err := client.GetInvoice(t.Context(), &pb.GetInvoiceRequest{
					Amount: invoiceAmount,
				})
				if err != nil {
					errs.add(err)
					return
				}
				err = lndPayInvoice(t.Context(), invoice.GetInvoice())
				errs.add(err)
			}()
			wg.Wait()

			require.Len(t, errs.errs, 2)
			require.NoError(t, errs.errs[0])
			require.NoError(t, errs.errs[1])
		})
	})

	t.Run("invalid", func(t *testing.T) {
		t.Run("same submarine swap", func(t *testing.T) {
			invoiceAmount := 2003
			invoice, _, err := lndAddInvoice(t.Context(), invoiceAmount)
			require.NoError(t, err)
			require.NotEmpty(t, invoice)

			wg := &sync.WaitGroup{}
			wg.Add(2)

			errs := errs{
				mu:   &sync.Mutex{},
				errs: make([]error, 0, 2),
			}
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
					Invoice: invoice,
				})
				errs.add(err)
			}()
			go func() {
				defer wg.Done()
				client, err := newFulmineClient("localhost:7000")
				if err != nil {
					errs.add(err)
					return
				}
				_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
					Invoice: invoice,
				})
				errs.add(err)
			}()
			wg.Wait()

			require.Len(t, errs.errs, 2)
			errCount := 0
			for _, err := range errs.errs {
				if err != nil {
					errCount++
				}
			}
			require.Equal(t, 1, errCount)
		})
	})
}

type errs struct {
	mu   *sync.Mutex
	errs []error
}

func (e *errs) add(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, err)
}

// TODO: add tests for increase inbound/outbound
