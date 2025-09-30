package swap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	"github.com/arkade-os/go-sdk/indexer"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/ccoveille/go-safecast"
	"github.com/lightningnetwork/lnd/input"

	log "github.com/sirupsen/logrus"
)

var ErrorNoVtxosFound = fmt.Errorf("no vtxos found for the given vhtlc opts")

type SwapHandler struct {
	arkClient       arksdk.ArkClient
	transportClient client.TransportClient
	indexerClient   indexer.Indexer
	boltzSvc        *boltz.Api
	publicKey       *btcec.PublicKey
	timeout         uint32
}

type SwapStatus int

const (
	SwapPending SwapStatus = iota
	SwapFailed
	SwapSuccess
)

type Swap struct {
	Id           string
	Invoice      string
	TxId         string
	Timestamp    int64
	RedeemTxid   string
	Status       SwapStatus
	PreimageHash []byte
	TimeoutInfo  boltz.TimeoutBlockHeights
	Opts         *vhtlc.Opts
	Amount       uint64
}

func NewSwapHandler(arkClient arksdk.ArkClient, transportClient client.TransportClient, indexerClient indexer.Indexer, boltzSvc *boltz.Api, publicKey *btcec.PublicKey, timeout uint32) *SwapHandler {

	println(timeout)
	return &SwapHandler{
		arkClient:       arkClient,
		transportClient: transportClient,
		indexerClient:   indexerClient,
		boltzSvc:        boltzSvc,
		publicKey:       publicKey,
		timeout:         timeout,
	}
}

func (h *SwapHandler) PayInvoice(ctx context.Context, invoice string, unilateralRefund func(swap Swap) error) (Swap, error) {
	if len(invoice) <= 0 {
		return Swap{}, fmt.Errorf("missing invoice")
	}

	return h.submarineSwap(ctx, invoice, unilateralRefund)
}

func (h *SwapHandler) PayOffer(ctx context.Context, offer string, lightningUrl string, unilateralRefund func(swap Swap) error) (Swap, error) {
	// Decode the offer to get the amount
	decodedOffer, err := DecodeBolt12Offer(offer)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode offer: %v", err)
	}

	amountInSats := decodedOffer.AmountInSats

	if amountInSats == 0 {
		return Swap{}, fmt.Errorf("offer amount is 0")
	}

	boltzApi := h.boltzSvc
	if lightningUrl != "" {
		boltzApi = &boltz.Api{
			URL: lightningUrl,
		}
	}

	response, err := boltzApi.FetchBolt12Invoice(boltz.FetchBolt12InvoiceRequest{
		Offer:  offer,
		Amount: amountInSats,
		Note:   decodedOffer.DescriptionStr,
	})

	if err != nil {
		return Swap{}, fmt.Errorf("failed to fetch invoice: %v", err)
	}

	if response.Error != "" {
		return Swap{}, fmt.Errorf("failed to fetch invoice: %s", response.Error)
	}

	return h.submarineSwap(ctx, response.Invoice, unilateralRefund)
}

// TODO (Joshua) : Ensure That this is being tracked
func (h *SwapHandler) GetInvoice(ctx context.Context, amount uint64, postProcess func(swap Swap) error) (Swap, error) {
	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return Swap{}, fmt.Errorf("failed to generate preimage: %w", err)
	}

	return h.reverseSwap(ctx, amount, preimage, postProcess)
}

func (h *SwapHandler) submarineSwap(ctx context.Context, invoice string, unilateralRefund func(swap Swap) error) (Swap, error) {
	if len(invoice) == 0 {
		return Swap{}, fmt.Errorf("invoice must not be empty")
	}

	var preimageHash []byte

	if IsBolt12Invoice(invoice) {
		decodedInvoice, err := DecodeBolt12Invoice(invoice)
		if err != nil {
			return Swap{}, fmt.Errorf("failed to decode bolt12 invoice: %v", err)
		}
		preimageHash = decodedInvoice.PaymentHash160
	} else {
		_, hash, err := DecodeInvoice(invoice)
		if err != nil {
			return Swap{}, fmt.Errorf("failed to decode invoice: %v", err)
		}
		preimageHash = hash
	}

	// Create the swap
	swap, err := h.boltzSvc.CreateSwap(boltz.CreateSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		Invoice:         invoice,
		RefundPublicKey: hex.EncodeToString(h.publicKey.SerializeCompressed()),
		PaymentTimeout:  h.timeout,
	})
	if err != nil {
		return Swap{}, fmt.Errorf("failed to make submarine swap: %v", err)
	}

	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return Swap{}, fmt.Errorf("invalid claim pubkey: %v", err)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	unilateralClaimDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim}
	unilateralRefundDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund}
	unilateralRefundWithoutReceiverDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver}

	vhtlcAddress, _, vhtlcOpts, err := h.getVHTLC(
		ctx,
		receiverPubkey,
		nil,
		preimageHash,
		refundLocktime,
		unilateralClaimDelay,
		unilateralRefundDelay,
		unilateralRefundWithoutReceiverDelay,
	)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != vhtlcAddress {
		return Swap{}, fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	contextTimeout := time.Second * time.Duration(h.timeout*2)

	ws := h.boltzSvc.NewWebsocket()
	ctx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()

	err = ws.ConnectAndSubscribe(ctx, []string{swap.Id}, 5*time.Second)
	if err != nil {
		return Swap{}, err
	}

	// Fund the VHTLC
	receivers := []types.Receiver{{To: swap.Address, Amount: swap.ExpectedAmount}}
	txid, err := h.arkClient.SendOffChain(ctx, false, receivers)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to pay to vHTLC address: %v", err)
	}

	swapDetails := Swap{
		Id:           swap.Id,
		Invoice:      invoice,
		TxId:         txid,
		PreimageHash: preimageHash,
		Timestamp:    time.Now().Unix(),
		TimeoutInfo:  swap.TimeoutBlockHeights,
		Status:       SwapPending,
		Opts:         vhtlcOpts,
		Amount:       swap.ExpectedAmount,
	}

	for {
		select {
		case update, ok := <-ws.Updates:
			// TODO (Joshua) : This should wait for payment to succeed, even after updates fail
			if !ok {
				return swapDetails, fmt.Errorf("updates closed")
			}
			switch boltz.ParseEvent(update.Status) {
			case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
				// Refund the VHTLC if the swap fails
				withReceiver := true
				swapDetails.Status = SwapFailed

				txid, err := h.refundVHTLC(
					context.Background(), swap.Id, withReceiver, *vhtlcOpts)

				if err != nil {
					go func() {
						err := unilateralRefund(swapDetails)
						if err != nil {
							log.WithError(err).Error("failed to do unilateral refund")
						}
					}()
				}
				swapDetails.RedeemTxid = txid

				return swapDetails, nil
			case boltz.TransactionClaimed, boltz.InvoiceSettled:
				swapDetails.Status = SwapSuccess

				return swapDetails, nil
			}
		case <-ctx.Done():
			swapDetails.Status = SwapFailed

			go func() {
				err := unilateralRefund(swapDetails)
				if err != nil {
					log.WithError(err).Error("failed to do unilateral refund")
				}
			}()

			return swapDetails, nil
		}
	}

}

func (h *SwapHandler) getVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey,
	preimageHash []byte,
	refundLocktime arklib.AbsoluteLocktime,
	unilateralClaimDelay arklib.RelativeLocktime,
	unilateralRefundDelay arklib.RelativeLocktime,
	unilateralRefundWithoutReceiverDelay arklib.RelativeLocktime,
) (string, *vhtlc.VHTLCScript, *vhtlc.Opts, error) {
	receiverPubkeySet := receiverPubkey != nil
	senderPubkeySet := senderPubkey != nil
	if receiverPubkeySet == senderPubkeySet {
		return "", nil, nil, fmt.Errorf("only one of receiver and sender pubkey must be set")
	}
	if !receiverPubkeySet {
		receiverPubkey = h.publicKey
	}
	if !senderPubkeySet {
		senderPubkey = h.publicKey
	}

	config, err := h.arkClient.GetConfigData(ctx)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to get config data: %v", err)
	}

	opts := vhtlc.Opts{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		Server:                               config.SignerPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}

	vHTLC, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", nil, nil, err
	}

	encodedAddr, err := vHTLC.Address(config.Network.Addr, config.SignerPubKey)
	if err != nil {
		return "", nil, nil, err
	}

	return encodedAddr, vHTLC, &opts, nil
}

func (h *SwapHandler) refundVHTLC(
	ctx context.Context, swapId string, withReceiver bool, vhtlcOpts vhtlc.Opts,
) (string, error) {
	cfg, err := h.arkClient.GetConfigData(ctx)
	if err != nil {
		return "", err
	}

	vtxos, err := h.getVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return "", err
	}

	if len(vtxos) == 0 {
		return "", fmt.Errorf("no vtxos found for the given vhtlc opts: %v", vhtlcOpts)
	}

	vtxo := vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	refundTapscript, err := vtxoScript.RefundTapscript(withReceiver)
	if err != nil {
		return "", err
	}

	_, offchainAddress, _, err := h.arkClient.Receive(ctx)
	if err != nil {
		return "", err
	}

	offchainPkScript, err := offchainAddressPkScript(offchainAddress)
	if err != nil {

		return "", err
	}

	dest, err := hex.DecodeString(offchainPkScript)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	refundTx, checkpointPtxs, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          refundTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: dest,
			},
		},
		checkpointExitScript(cfg),
	)
	if err != nil {
		return "", err
	}

	refundTxStr, err := refundTx.B64Encode()
	if err != nil {
		return "", err
	}

	signedRefundTx, err := h.arkClient.SignTransaction(ctx, refundTxStr)
	if err != nil {
		return "", err
	}

	if withReceiver {
		signedRefundTx, err = h.boltzRefundSwap(swapId, signedRefundTx)
		if err != nil {
			return "", err
		}
	}

	checkpointTxs := make([]string, 0, len(checkpointPtxs))
	for _, ptx := range checkpointPtxs {
		tx, err := ptx.B64Encode()
		if err != nil {
			return "", err
		}
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, signedCheckpoints, err := h.transportClient.SubmitTx(ctx, signedRefundTx, checkpointTxs)
	if err != nil {
		return "", err
	}

	if err := verifyFinalArkTx(finalArkTx, cfg.SignerPubKey, getInputTapLeaves(refundTx)); err != nil {
		return "", err
	}

	// verify and sign the checkpoints
	signCheckpoint := func(tx *psbt.Packet) (string, error) {
		encoded, err := tx.B64Encode()
		if err != nil {
			return "", err
		}
		return h.arkClient.SignTransaction(ctx, encoded)
	}

	finalCheckpoints, err := verifyAndSignCheckpoints(signedCheckpoints, checkpointPtxs, cfg.SignerPubKey, signCheckpoint)
	if err != nil {
		return "", err
	}

	err = h.transportClient.FinalizeTx(ctx, arkTxid, finalCheckpoints)
	if err != nil {
		return "", fmt.Errorf("failed to finalize redeem transaction: %w", err)
	}

	return arkTxid, nil
}

func (h *SwapHandler) boltzRefundSwap(swapId, refundTx string) (string, error) {
	tx, err := h.boltzSvc.RefundSubmarine(swapId, boltz.RefundSwapRequest{
		Transaction: refundTx,
	})
	if err != nil {
		return "", err
	}

	return tx.Transaction, nil
}

func (h *SwapHandler) reverseSwap(ctx context.Context, amount uint64, preimage []byte, postProcess func(swap Swap) error) (Swap, error) {
	var preimageHash []byte
	buf := sha256.Sum256(preimage)
	preimageHash = input.Ripemd160H(buf[:])

	swap, err := h.boltzSvc.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           boltz.CurrencyBtc,
		To:             boltz.CurrencyArk,
		InvoiceAmount:  amount,
		ClaimPublicKey: hex.EncodeToString(h.publicKey.SerializeCompressed()),
		PreimageHash:   hex.EncodeToString(buf[:]),
	})

	if err != nil {
		return Swap{}, fmt.Errorf("failed to make reverse submarine swap: %v", err)
	}

	// verify vHTLC
	senderPubkey, err := parsePubkey(swap.RefundPublicKey)
	if err != nil {
		return Swap{}, fmt.Errorf("invalid refund pubkey: %v", err)
	}

	// verify preimage hash and invoice amount
	invoiceAmount, gotPreimageHash, err := DecodeInvoice(swap.Invoice)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode invoice: %v", err)
	}

	if !bytes.Equal(preimageHash, gotPreimageHash) {
		return Swap{}, fmt.Errorf("invalid preimage hash: expected %x, got %x", preimageHash, gotPreimageHash)
	}
	if invoiceAmount != amount {
		return Swap{}, fmt.Errorf("invalid invoice amount: expected %d, got %d", amount, invoiceAmount)
	}

	refundLocktime := arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime)
	unilateralClaimDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim}
	unilateralRefundDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund}
	unilateralRefundWithoutReceiverDelay := arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver}

	vhtlcAddress, _, vhtlcOpts, err := h.getVHTLC(
		ctx,
		nil,
		senderPubkey,
		gotPreimageHash,
		refundLocktime,
		unilateralClaimDelay,
		unilateralRefundDelay,
		unilateralRefundWithoutReceiverDelay,
	)

	swapDetails := Swap{
		Id:           swap.Id,
		Invoice:      swap.Invoice,
		PreimageHash: preimageHash,
		TimeoutInfo:  swap.TimeoutBlockHeights,
		Timestamp:    time.Now().Unix(),
		Status:       SwapPending,
		Amount:       amount,
		Opts:         vhtlcOpts,
	}

	if err != nil {
		return swapDetails, fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return swapDetails, fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// TODO: (Joshua) This should exists for the lifetime of the invoice
	go func() {
		if reedeemTxId, err := h.waitAndClaimVHTLC(
			context.Background(), swap.Id, preimage, vhtlcOpts,
		); err != nil {
			swapDetails.Status = SwapFailed
			log.WithError(err).Error("failed to claim VHTLC")
		} else {
			swapDetails.RedeemTxid = reedeemTxId
			swapDetails.Status = SwapSuccess
		}

		err = postProcess(swapDetails)
		if err != nil {
			log.WithError(err).Error("failed to post process swap")
		}
	}()
	return swapDetails, nil
}

func (h *SwapHandler) waitAndClaimVHTLC(
	ctx context.Context, swapId string, preimage []byte, vhtlcOpts *vhtlc.Opts,
) (string, error) {
	ws := h.boltzSvc.NewWebsocket()
	defer ws.Close()
	{
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		err := ws.Connect()
		for err != nil {
			log.WithError(err).Warn("failed to connect to boltz websocket")
			time.Sleep(time.Second)
			log.Debug("reconnecting...")
			err = ws.Connect()
			if ctx.Err() != nil {
				return "", fmt.Errorf("timeout while connecting to websocket: %v", ctx.Err())
			}
		}

		err = ws.Subscribe([]string{swapId})
		for err != nil {
			log.WithError(err).Warn("failed to subscribe for swap events")
			time.Sleep(time.Second)
			log.Debug("retrying...")
			err = ws.Subscribe([]string{swapId})
		}
	}

	var txid string
	for update := range ws.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		confirmed := false
		switch parsedStatus {
		case boltz.TransactionMempool:
			confirmed = true
		case boltz.InvoiceFailedToPay, boltz.TransactionFailed, boltz.TransactionLockupFailed:
			return "", fmt.Errorf("failed to receive payment: %s", update.Status)
		}
		if confirmed {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			interval := 200 * time.Millisecond
			log.Debug("claiming VHTLC with preimage...")
			if err := Retry(ctx, interval, func(ctx context.Context) (bool, error) {
				var err error
				txid, err = h.claimVHTLC(ctx, preimage, *vhtlcOpts)
				if err != nil {
					if errors.Is(err, ErrorNoVtxosFound) {
						return false, nil
					}
					return false, err
				}

				return true, nil
			}); err != nil {
				return "", err
			}
			log.Debugf("successfully claimed VHTLC with tx: %s", txid)
			break
		}
	}

	return txid, nil
}

func (h *SwapHandler) getVHTLCFunds(ctx context.Context, vhtlcOpts vhtlc.Opts) ([]types.Vtxo, error) {
	vHTLC, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return nil, err
	}

	tapKey, _, err := vHTLC.TapTree()
	if err != nil {
		return nil, err
	}

	outScript, err := script.P2TRScript(tapKey)
	if err != nil {
		return nil, err
	}

	vtxosRequest := indexer.GetVtxosRequestOption{}
	if err := vtxosRequest.WithScripts([]string{hex.EncodeToString(outScript)}); err != nil {
		return nil, err
	}
	resp, err := h.indexerClient.GetVtxos(ctx, vtxosRequest)
	if err != nil {
		return nil, err
	}

	return resp.Vtxos, nil
}

func (h *SwapHandler) claimVHTLC(
	ctx context.Context, preimage []byte, vhtlcOpts vhtlc.Opts,
) (string, error) {
	vtxos, err := h.getVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return "", err
	}
	if len(vtxos) == 0 {
		return "", ErrorNoVtxosFound
	}

	vtxo := &vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	// self send output
	_, myAddr, _, err := h.arkClient.Receive(ctx)
	if err != nil {
		return "", err
	}

	decodedAddr, err := arklib.DecodeAddressV0(myAddr)
	if err != nil {
		return "", err
	}

	pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	cfg, err := h.arkClient.GetConfigData(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get config data: %w", err)
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	claimTapscript, err := vtxoScript.ClaimTapscript()
	if err != nil {
		return "", err
	}

	arkTx, checkpoints, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          claimTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
		checkpointExitScript(cfg),
	)
	if err != nil {
		return "", err
	}

	signTransaction := func(tx *psbt.Packet) (string, error) {
		// add the preimage to the checkpoint input
		if err := txutils.AddConditionWitness(0, tx, wire.TxWitness{preimage}); err != nil {
			return "", err
		}

		encoded, err := tx.B64Encode()
		if err != nil {
			return "", err
		}

		return h.arkClient.SignTransaction(ctx, encoded)
	}

	signedArkTx, err := signTransaction(arkTx)
	if err != nil {
		return "", err
	}

	checkpointTxs := make([]string, 0, len(checkpoints))
	for _, ptx := range checkpoints {
		tx, err := ptx.B64Encode()
		if err != nil {
			return "", err
		}
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, signedCheckpoints, err := h.transportClient.SubmitTx(ctx, signedArkTx, checkpointTxs)
	if err != nil {
		return "", err
	}

	if err := verifyFinalArkTx(finalArkTx, cfg.SignerPubKey, getInputTapLeaves(arkTx)); err != nil {
		return "", err
	}

	finalCheckpoints, err := verifyAndSignCheckpoints(signedCheckpoints, checkpoints, cfg.SignerPubKey, signTransaction)
	if err != nil {
		return "", err
	}

	err = h.transportClient.FinalizeTx(ctx, arkTxid, finalCheckpoints)
	if err != nil {
		return "", fmt.Errorf("failed to finalize redeem transaction: %w", err)
	}

	return arkTxid, nil
}

func checkpointExitScript(cfg *types.Config) *script.CSVMultisigClosure {
	return &script.CSVMultisigClosure{
		Locktime: cfg.UnilateralExitDelay,
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{cfg.SignerPubKey},
		},
	}
}

// verifyInputSignatures checks that all inputs have a signature for the given pubkey
// and the signature is correct for the given tapscript leaf
func verifyInputSignatures(tx *psbt.Packet, pubkey *btcec.PublicKey, tapLeaves map[int]txscript.TapLeaf) error {
	xOnlyPubkey := schnorr.SerializePubKey(pubkey)

	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	sigsToVerify := make(map[int]*psbt.TaprootScriptSpendSig)

	for inputIndex, input := range tx.Inputs {
		// collect previous outputs
		if input.WitnessUtxo == nil {
			return fmt.Errorf("input %d has no witness utxo, cannot verify signature", inputIndex)
		}

		outpoint := tx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint
		prevouts[outpoint] = input.WitnessUtxo

		tapLeaf, ok := tapLeaves[inputIndex]
		if !ok {
			return fmt.Errorf("input %d has no tapscript leaf, cannot verify signature", inputIndex)
		}

		tapLeafHash := tapLeaf.TapHash()

		// check if pubkey has a tapscript sig
		hasSig := false
		for _, sig := range input.TaprootScriptSpendSig {
			if bytes.Equal(sig.XOnlyPubKey, xOnlyPubkey) && bytes.Equal(sig.LeafHash, tapLeafHash[:]) {
				hasSig = true
				sigsToVerify[inputIndex] = sig
				break
			}
		}

		if !hasSig {
			return fmt.Errorf("input %d has no signature for pubkey %x", inputIndex, xOnlyPubkey)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txSigHashes := txscript.NewTxSigHashes(tx.UnsignedTx, prevoutFetcher)

	for inputIndex, sig := range sigsToVerify {
		msgHash, err := txscript.CalcTapscriptSignaturehash(
			txSigHashes,
			sig.SigHash,
			tx.UnsignedTx,
			inputIndex,
			prevoutFetcher,
			tapLeaves[inputIndex],
		)
		if err != nil {
			return fmt.Errorf("failed to calculate tapscript signature hash: %w", err)
		}

		signature, err := schnorr.ParseSignature(sig.Signature)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		if !signature.Verify(msgHash, pubkey) {
			return fmt.Errorf("input %d: invalid signature", inputIndex)
		}
	}

	return nil
}

// GetInputTapLeaves returns a map of input index to tapscript leaf
// if the input has no tapscript leaf, it is not included in the map
func getInputTapLeaves(tx *psbt.Packet) map[int]txscript.TapLeaf {
	tapLeaves := make(map[int]txscript.TapLeaf)
	for inputIndex, input := range tx.Inputs {
		if input.TaprootLeafScript == nil {
			continue
		}
		tapLeaves[inputIndex] = txscript.NewBaseTapLeaf(input.TaprootLeafScript[0].Script)
	}
	return tapLeaves
}

func verifyAndSignCheckpoints(signedCheckpoints []string, myCheckpoints []*psbt.Packet, arkSigner *btcec.PublicKey, sign func(tx *psbt.Packet) (string, error)) ([]string, error) {
	finalCheckpoints := make([]string, 0, len(signedCheckpoints))
	for _, checkpoint := range signedCheckpoints {
		signedCheckpointPtx, err := psbt.NewFromRawBytes(strings.NewReader(checkpoint), true)
		if err != nil {
			return nil, err
		}

		// search for the checkpoint tx we initially created
		var myCheckpointTx *psbt.Packet
		for _, chk := range myCheckpoints {
			if chk.UnsignedTx.TxID() == signedCheckpointPtx.UnsignedTx.TxID() {
				myCheckpointTx = chk
				break
			}
		}
		if myCheckpointTx == nil {
			return nil, fmt.Errorf("checkpoint tx not found")
		}

		// verify the server has signed the checkpoint tx
		err = verifyInputSignatures(signedCheckpointPtx, arkSigner, getInputTapLeaves(myCheckpointTx))
		if err != nil {
			return nil, err
		}

		finalCheckpoint, err := sign(signedCheckpointPtx)
		if err != nil {
			return nil, fmt.Errorf("failed to sign checkpoint transaction: %w", err)
		}

		finalCheckpoints = append(finalCheckpoints, finalCheckpoint)
	}

	return finalCheckpoints, nil
}

func verifyFinalArkTx(finalArkTx string, arkSigner *btcec.PublicKey, expectedTapLeaves map[int]txscript.TapLeaf) error {
	finalArkPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalArkTx), true)
	if err != nil {
		return err
	}

	// verify that the ark signer has signed the ark tx
	err = verifyInputSignatures(finalArkPtx, arkSigner, expectedTapLeaves)
	if err != nil {
		return err
	}

	return nil
}

func offchainAddressPkScript(addr string) (string, error) {
	decodedAddress, err := arklib.DecodeAddressV0(addr)
	if err != nil {
		return "", fmt.Errorf("failed to decode address %s: %w", addr, err)
	}

	p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse address to p2tr script: %w", err)
	}
	return hex.EncodeToString(p2trScript), nil
}
