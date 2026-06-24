package e2e_test

import (
	"encoding/hex"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/input"
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

	t.Run("refund", func(t *testing.T) {
		invoice, rHash, err := lndAddInvoice(t.Context(), 5000)
		require.NoError(t, err)
		require.NotEmpty(t, invoice)
		require.NotEmpty(t, rHash)

		err = lndCancelInvoice(t.Context(), rHash)
		require.NoError(t, err)

		balance, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balance)
		require.Greater(t, int(balance.GetAmount()), 5000)

		_, err = client.PayInvoice(t.Context(), &pb.PayInvoiceRequest{
			Invoice: invoice,
		})
		require.NoError(t, err)

		// Give the time to update after the refund
		time.Sleep(5 * time.Second)

		balanceAfter, err := client.GetBalance(t.Context(), &pb.GetBalanceRequest{})
		require.NoError(t, err)
		require.NotNil(t, balanceAfter)
		before := int64(balance.GetAmount())
		after := int64(balanceAfter.GetAmount())
		require.Zero(t, before-after)
	})
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

func TestSubmarineSwapUnderfundedCooperativeRefund(t *testing.T) {
	ctx := t.Context()

	arkClient, arkPubKey, _ := setupArkSDKwithPublicKey(t)
	faucetOffchain(t, arkClient, 0.001)

	cfg, err := arkClient.GetConfigData(ctx)
	require.NoError(t, err)

	handlerKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	boltzApi := &boltz.Api{URL: "http://localhost:9001", WSURL: "ws://localhost:9004"}

	invoice, rHash, err := lndAddInvoice(ctx, 5000)
	require.NoError(t, err)
	require.NotEmpty(t, invoice)

	preimageHash := input.Ripemd160H(mustDecodeHex(t, rHash))

	createResp, err := boltzApi.CreateSwap(boltz.CreateSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		Invoice:         invoice,
		RefundPublicKey: hex.EncodeToString(arkPubKey.SerializeCompressed()),
		PaymentTimeout:  120,
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Address)
	require.Greater(t, createResp.ExpectedAmount, uint64(100))

	receiverPubBytes, err := hex.DecodeString(createResp.ClaimPublicKey)
	require.NoError(t, err)
	receiverPub, err := btcec.ParsePubKey(receiverPubBytes)
	require.NoError(t, err)

	opts := vhtlc.Opts{
		Sender:                               arkPubKey,
		Receiver:                             receiverPub,
		Server:                               cfg.SignerPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       arklib.AbsoluteLocktime(createResp.TimeoutBlockHeights.RefundLocktime),
		UnilateralClaimDelay:                 boltzLocktime(createResp.TimeoutBlockHeights.UnilateralClaim),
		UnilateralRefundDelay:                boltzLocktime(createResp.TimeoutBlockHeights.UnilateralRefund),
		UnilateralRefundWithoutReceiverDelay: boltzLocktime(createResp.TimeoutBlockHeights.UnilateralRefundWithoutReceiver),
	}
	script, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	require.NoError(t, err)
	addr, err := script.Address(cfg.Network.Addr)
	require.NoError(t, err)
	require.Equal(t, createResp.Address, addr, "locally derived VHTLC address must match Boltz's")

	short := createResp.ExpectedAmount - 100
	_, err = arkClient.SendOffChain(ctx, []clientTypes.Receiver{
		{To: createResp.Address, Amount: short},
	})
	require.NoError(t, err)

	// Give Boltz time to observe the underfunded lockup tx and mark the swap
	// as failed before we ask it to co-sign the refund.
	time.Sleep(5 * time.Second)

	handler, err := swap.NewSwapHandler(arkClient, boltzApi, "", handlerKey, 120)
	require.NoError(t, err)

	_, err = handler.RefundSwap(ctx, swap.SwapTypeSubmarine, createResp.Id, true, opts, nil)
	require.NoError(t, err, "cooperative refund of an underfunded submarine swap should succeed")
}

func boltzLocktime(value uint32) arklib.RelativeLocktime {
	if value >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: value}
	}
	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: value}
}

// TODO: add tests for increase inbound/outbound
