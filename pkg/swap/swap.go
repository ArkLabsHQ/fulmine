package swap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	client "github.com/arkade-os/arkd/pkg/client-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/ccoveille/go-safecast"
	"github.com/lightningnetwork/lnd/input"
	decodepay "github.com/nbd-wtf/ln-decodepay"

	log "github.com/sirupsen/logrus"
)

var ErrorNoVtxosFound = fmt.Errorf("no vtxos found for the given vhtlc opts")

type SwapHandler struct {
	arkClient      arksdk.ArkClient
	boltzSvc       *boltz.Api
	explorerClient ExplorerClient
	privateKey     *btcec.PrivateKey
	publicKey      *btcec.PublicKey
	timeout        uint32
	config         clientTypes.Config
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

func NewSwapHandler(
	arkClient arksdk.ArkClient,
	boltzSvc *boltz.Api,
	esploraURL string,
	privateKey *btcec.PrivateKey,
	timeout uint32,
) (*SwapHandler, error) {
	cfg, err := arkClient.GetConfigData(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get config data: %w", err)
	}
	return &SwapHandler{
		arkClient:      arkClient,
		boltzSvc:       boltzSvc,
		explorerClient: NewExplorerClient(esploraURL),
		privateKey:     privateKey,
		publicKey:      privateKey.PubKey(),
		timeout:        timeout,
		config:         *cfg,
	}, nil
}

func (h *SwapHandler) PayInvoice(
	ctx context.Context, invoice string, unilateralRefund func(swap Swap) error,
) (*Swap, error) {
	if len(invoice) <= 0 {
		return nil, fmt.Errorf("missing invoice")
	}

	return h.submarineSwap(ctx, invoice, unilateralRefund)
}

func (h *SwapHandler) PayOffer(
	ctx context.Context, offer string, lightningUrl string, unilateralRefund func(swap Swap) error,
) (*Swap, error) {
	// Decode the offer to get the amount
	decodedOffer, err := DecodeBolt12Offer(offer)
	if err != nil {
		return nil, fmt.Errorf("failed to decode offer: %v", err)
	}

	amountInSats := decodedOffer.AmountInSats
	if amountInSats == 0 {
		return nil, fmt.Errorf("offer amount must be greater than 0")
	}

	boltzApi := h.boltzSvc
	if lightningUrl != "" {
		boltzApi = &boltz.Api{URL: lightningUrl}
	}

	response, err := boltzApi.FetchBolt12Invoice(boltz.FetchBolt12InvoiceRequest{
		Offer:  offer,
		Amount: amountInSats,
		Note:   decodedOffer.DescriptionStr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch invoice: %v", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("failed to fetch invoice: %s", response.Error)
	}

	return h.submarineSwap(ctx, response.Invoice, unilateralRefund)
}

func (h *SwapHandler) GetInvoice(
	ctx context.Context, amount uint64, postProcess func(swap Swap) error,
) (Swap, error) {
	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return Swap{}, fmt.Errorf("failed to generate preimage: %w", err)
	}

	return h.reverseSwap(ctx, amount, preimage, postProcess)
}

func (h *SwapHandler) GetVHTLCFunds(
	ctx context.Context, vhtlcOpts []vhtlc.Opts,
) ([]clientTypes.Vtxo, error) {
	vHTLCs := make([]*vhtlc.VHTLCScript, 0, len(vhtlcOpts))
	for _, opts := range vhtlcOpts {
		vHTLC, err := vhtlc.NewVHTLCScriptFromOpts(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse VHTLC from opts: %w", err)
		}
		vHTLCs = append(vHTLCs, vHTLC)
	}

	return h.getVHTLCFunds(ctx, vHTLCs)
}

func (h *SwapHandler) ClaimVHTLC(
	ctx context.Context, preimage []byte, vhtlcOpts vhtlc.Opts, outpoint *clientTypes.Outpoint,
) (string, error) {
	vHTLC, err := vhtlc.NewVHTLCScriptFromOpts(vhtlcOpts)
	if err != nil {
		return "", err
	}

	vtxo, pending, err := h.selectClaimableVTXO(ctx, vHTLC, outpoint)
	if err != nil {
		return "", err
	}
	if pending {
		txids, err := h.arkClient.FinalizePendingTxs(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to finalize pending txs: %w", err)
		}
		if len(txids) > 0 {
			return txids[0], nil
		}
		return "", fmt.Errorf("vtxo is pending but no pending txs found to finalize")
	}

	//this is safety net for Boltz Fulmine if VTXO is recoverable in the moment of Claim
	if vtxo.IsRecoverable() && vtxo.Amount >= h.config.Dust {
		txid, err := h.SettleVHTLCWithClaimPath(ctx, vhtlcOpts, preimage, &vtxo.Outpoint)
		if err != nil {
			return "", fmt.Errorf("failed to settle vhtlc with claim path: %w", err)
		}

		log.Infof("recoverable vhtlc settled with claim path: %s", txid)
		return txid, nil
	}

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}
	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	// self send output
	myAddr, err := h.arkClient.NewOffchainAddress(ctx)
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

	claimTapscript, err := vHTLC.ClaimTapscript()
	if err != nil {
		return "", err
	}

	arkTx, checkpoints, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vHTLC.GetRevealedTapscripts(),
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
		checkpointExitScript(h.config),
	)
	if err != nil {
		return "", err
	}

	signTransaction := func(tx *psbt.Packet) (string, error) {
		// add the preimage to the checkpoint input
		if err := txutils.SetArkPsbtField(
			tx, 0, txutils.ConditionWitnessField, wire.TxWitness{preimage},
		); err != nil {
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

	arkTxid, finalArkTx, signedCheckpoints, err := h.arkClient.Client().SubmitTx(
		ctx, signedArkTx, checkpointTxs,
	)
	if err != nil {
		return "", err
	}

	if err := verifyFinalArkTx(
		finalArkTx, h.config.SignerPubKey, getInputTapLeaves(arkTx),
	); err != nil {
		return "", err
	}

	finalCheckpoints, err := verifyAndSignCheckpoints(
		signedCheckpoints, checkpoints, h.config.SignerPubKey, signTransaction,
	)
	if err != nil {
		return "", err
	}

	if err := h.arkClient.Client().FinalizeTx(ctx, arkTxid, finalCheckpoints); err != nil {
		return "", err
	}

	return arkTxid, nil
}

func (h *SwapHandler) RefundSwap(
	ctx context.Context, swapType, swapId string, withReceiver bool, vhtlcOpts vhtlc.Opts,
	outpoint *clientTypes.Outpoint,
) (string, error) {
	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(vhtlcOpts)
	if err != nil {
		return "", err
	}

	vtxo, pending, err := h.selectClaimableVTXO(ctx, vhtlcScript, outpoint)
	if err != nil {
		return "", err
	}
	if pending {
		txids, err := h.arkClient.FinalizePendingTxs(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to finalize pending txs: %w", err)
		}
		if len(txids) > 0 {
			return txids[0], nil
		}
		return "", fmt.Errorf("vtxo is pending but no pending txs found to finalize")
	}

	//this is safety net for Boltz Fulmine if VTXO is recoverable in the moment of Refund
	if vtxo.IsRecoverable() && vtxo.Amount >= h.config.Dust {
		txid, err := h.SettleVhtlcWithRefundPath(ctx, vhtlcOpts, &vtxo.Outpoint)
		if err != nil {
			return "", fmt.Errorf("failed to settle vhtlc with refund path: %w", err)
		}

		log.Infof("recoverable vhtlc settled with refund path: %s", txid)
		return txid, nil
	}

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}
	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	refundTapscript, err := vhtlcScript.RefundTapscript(withReceiver)
	if err != nil {
		return "", err
	}

	offchainAddress, err := h.arkClient.NewOffchainAddress(ctx)
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
				RevealedTapscripts: vhtlcScript.GetRevealedTapscripts(),
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
		checkpointExitScript(h.config),
	)
	if err != nil {
		return "", err
	}

	if len(checkpointPtxs) != 1 {
		return "", fmt.Errorf(
			"failed to build refund tx: expected 1 checkpoint tx got %d", len(checkpointPtxs),
		)
	}
	unsignedRefundTx, err := refundTx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode unsigned refund tx: %s", err)
	}
	unsignedCheckpointTx, err := checkpointPtxs[0].B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode unsigned refund checkpoint tx: %s", err)
	}

	signTransaction := func(tx *psbt.Packet) (string, error) {
		encoded, err := tx.B64Encode()
		if err != nil {
			return "", err
		}
		return h.arkClient.SignTransaction(ctx, encoded)
	}

	// user signing
	signedRefundTx, err := signTransaction(refundTx)
	if err != nil {
		return "", fmt.Errorf("failed to sign refund tx: %s", err)
	}
	signedCheckpointTx, err := signTransaction(checkpointPtxs[0])
	if err != nil {
		return "", fmt.Errorf("failed to sign refund checkpoint tx: %s", err)
	}

	signedRefundPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedRefundTx), true)
	if err != nil {
		return "", fmt.Errorf("failed to decode refund tx signed by us: %s", err)
	}

	signedCheckpointPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedCheckpointTx), true)
	if err != nil {
		return "", fmt.Errorf("failed to decode checkpoint tx signed by us: %s", err)
	}

	pubKeysToVerify := []*btcec.PublicKey{vhtlcOpts.Sender, vhtlcOpts.Server}
	checkpointsList := append([]*psbt.Packet{}, signedCheckpointPsbt)

	// if withReceiver is enabled, boltz should sign the transactions
	if withReceiver {
		pubKeysToVerify = append(pubKeysToVerify, vhtlcOpts.Receiver)

		// Determine which refund function to use based on swap type
		var refundFunc func(string, boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error)
		switch swapType {
		case SwapTypeSubmarine:
			refundFunc = h.boltzSvc.RefundSubmarine
		case SwapTypeChain:
			refundFunc = h.boltzSvc.RefundChainSwap
		default:
			return "", fmt.Errorf("unsupported swap type for collaborative refund: %s", swapType)
		}

		boltzSignedRefundPtx, boltzSignedCheckpointPtx, err := h.collaborativeRefund(
			refundFunc, swapId, unsignedRefundTx, unsignedCheckpointTx)

		if err != nil {
			return "", err
		}

		for i := range signedRefundPsbt.Inputs {
			boltzIn := boltzSignedRefundPtx.Inputs[i]
			partialSig := boltzIn.TaprootScriptSpendSig[0]
			signedRefundPsbt.Inputs[i].TaprootScriptSpendSig =
				append(signedRefundPsbt.Inputs[i].TaprootScriptSpendSig, partialSig)
		}

		checkpointsList = append(checkpointsList, boltzSignedCheckpointPtx)

	}

	signedRefund, err := signedRefundPsbt.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode final refund tx: %s", err)
	}

	arkTxid, finalRefundTx, serverSignedCheckpoints, err := h.arkClient.Client().SubmitTx(
		ctx, signedRefund, []string{unsignedCheckpointTx},
	)
	if err != nil {
		return "", err
	}

	finalRefundPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalRefundTx), true)
	if err != nil {
		return "", fmt.Errorf("failed to decode refund tx signed by server: %s", err)
	}

	serverCheckpointPtx, err := psbt.NewFromRawBytes(
		strings.NewReader(serverSignedCheckpoints[0]), true,
	)
	if err != nil {
		return "", fmt.Errorf("failed to decode checkpoint tx signed by us: %s", err)
	}

	if err := verifySignatures(
		[]*psbt.Packet{finalRefundPtx}, pubKeysToVerify, getInputTapLeaves(refundTx),
	); err != nil {
		return "", err
	}

	// combine checkpoint Transactions
	checkpointsList = append(checkpointsList, serverCheckpointPtx)
	finalCheckpointPtx, err := combineTapscripts(checkpointsList)
	if err != nil {
		return "", fmt.Errorf("failed to combine checkpoint txs: %s", err)
	}

	if err := verifySignatures(
		[]*psbt.Packet{finalCheckpointPtx}, pubKeysToVerify,
		getInputTapLeaves(serverCheckpointPtx),
	); err != nil {
		return "", err
	}

	finalCheckpointTx, err := finalCheckpointPtx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode final checkpoint tx: %s", err)
	}

	if err := h.arkClient.Client().FinalizeTx(ctx, arkTxid, []string{finalCheckpointTx}); err != nil {
		return "", fmt.Errorf("failed to finalize refund tx: %w", err)
	}

	return arkTxid, nil
}

// SettleVHTLCWithClaimPath settles a VHTLC using the claim path (revealing preimage) via batch session.
// This is used for reverse submarine swaps where Fulmine is the receiver.
func (h *SwapHandler) SettleVHTLCWithClaimPath(
	ctx context.Context, vhtlcOpts vhtlc.Opts, preimage []byte, outpoint *clientTypes.Outpoint,
) (string, error) {
	if err := validatePreimage(preimage, vhtlcOpts.PreimageHash); err != nil {
		return "", err
	}

	session, err := h.getBatchSessionArgs(ctx, vhtlcOpts, outpoint, nil)
	if err != nil {
		return "", err
	}

	proof, message, err := getClaimIntent(session, preimage)
	if err != nil {
		return "", fmt.Errorf("failed to build claim intent: %w", err)
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, proof)
	if err != nil {
		return "", fmt.Errorf("failed to sign intent proof: %w", err)
	}

	intentID, err := h.arkClient.Client().RegisterIntent(ctx, signedProof, message)
	if err != nil {
		return "", fmt.Errorf("failed to register VHTLC claim intent: %w", err)
	}

	topics := getEventTopics(session.vtxos, session.signerSession.GetPublicKey())
	eventsCh, cancel, err := h.arkClient.Client().GetEventStream(ctx, topics)
	if err != nil {
		return "", fmt.Errorf("failed to get event stream: %w", err)
	}
	defer cancel()

	claimHandler, err := newClaimBatchSessionHandler(
		h.arkClient,
		intentID,
		session.vtxos,
		[]clientTypes.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}},
		preimage,
		map[string]*vhtlc.VHTLCScript{session.vtxos[0].Script: session.vhtlcScript},
		h.config,
		session.signerSession,
	)
	if err != nil {
		return "", fmt.Errorf("failed to setup claim batch session handler: %w", err)
	}

	txid, _, _, _, _, err := client.JoinBatchSession(ctx, eventsCh, claimHandler)
	if err != nil {
		return "", fmt.Errorf("batch session failed: %w", err)
	}

	log.Debugf("successfully claimed VHTLC in round %s", txid)
	return txid, nil
}

// SettleVhtlcWithRefundPath settles a VHTLC using the refund path via batch session.
// This is used for submarine swaps where Fulmine is the sender and needs to recover funds.
func (h *SwapHandler) SettleVhtlcWithRefundPath(
	ctx context.Context, vhtlcOpts vhtlc.Opts, outpoint *clientTypes.Outpoint,
) (string, error) {
	session, err := h.getBatchSessionArgs(ctx, vhtlcOpts, outpoint, nil)
	if err != nil {
		return "", err
	}

	proof, message, err := getRefundIntent(session)
	if err != nil {
		return "", fmt.Errorf("failed to build refund intent: %w", err)
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, proof)
	if err != nil {
		return "", fmt.Errorf("failed to sign intent proof: %w", err)
	}

	intentID, err := h.arkClient.Client().RegisterIntent(ctx, signedProof, message)
	if err != nil {
		return "", fmt.Errorf("failed to register VHTLC refund intent: %w", err)
	}

	topics := getEventTopics(session.vtxos, session.signerSession.GetPublicKey())
	eventsCh, cancel, err := h.arkClient.Client().GetEventStream(ctx, topics)
	if err != nil {
		return "", fmt.Errorf("failed to get event stream: %w", err)
	}
	defer cancel()

	withReceiver := true
	withoutReceiver := !withReceiver
	refundHandler, err := newRefundBatchSessionHandler(
		h.arkClient,
		h.arkClient.Client(),
		intentID,
		session.vtxos,
		[]clientTypes.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}},
		withoutReceiver,
		map[string]*vhtlc.VHTLCScript{session.vtxos[0].Script: session.vhtlcScript},
		h.config,
		h.publicKey,
		session.signerSession,
	)
	if err != nil {
		return "", fmt.Errorf("failed to setup refund batch session handler: %w", err)
	}

	txid, _, _, _, _, err := client.JoinBatchSession(ctx, eventsCh, refundHandler)
	if err != nil {
		return "", fmt.Errorf("batch session failed: %w", err)
	}

	log.Debugf("successfully refunded VHTLC in round %s", txid)
	return txid, nil
}

func (h *SwapHandler) SettleVHTLCWithCollaborativeRefundPath(
	ctx context.Context, vhtlcOpts vhtlc.Opts,
	partialForfeitTx, proof, message string, signerSession tree.SignerSession,
	outpoint *clientTypes.Outpoint,
) (string, error) {
	session, err := h.getBatchSessionArgs(ctx, vhtlcOpts, outpoint, &signerSession)
	if err != nil {
		return "", err
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, proof)
	if err != nil {
		return "", fmt.Errorf("failed to cosign intent proof: %w", err)
	}

	intentId, err := h.arkClient.Client().RegisterIntent(ctx, signedProof, message)
	if err != nil {
		return "", fmt.Errorf("failed to register intent: %w", err)
	}

	withReceiver := true
	handler, err := newCollabRefundBatchSessionHandler(
		h.arkClient,
		h.arkClient.Client(),
		intentId,
		session.vtxos,
		[]clientTypes.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}},
		withReceiver,
		map[string]*vhtlc.VHTLCScript{session.vtxos[0].Script: session.vhtlcScript},
		h.config,
		session.signerSession,
		partialForfeitTx,
	)
	if err != nil {
		return "", fmt.Errorf("failed to setup collab refund batch session handler: %w", err)
	}

	topics := getEventTopics(session.vtxos, session.signerSession.GetPublicKey())

	eventsCh, cancel, err := h.arkClient.Client().GetEventStream(ctx, topics)
	if err != nil {
		return "", fmt.Errorf("failed to get event stream: %w", err)
	}
	defer cancel()

	txid, _, _, _, _, err := client.JoinBatchSession(ctx, eventsCh, handler)
	if err != nil {
		return "", fmt.Errorf("batch session failed: %w", err)
	}

	log.Debugf("successfully completed delegate refund in round %s", txid)
	return txid, nil
}

func (h *SwapHandler) submarineSwap(
	ctx context.Context, invoice string, unilateralRefund func(swap Swap) error,
) (*Swap, error) {
	if len(invoice) == 0 {
		return nil, fmt.Errorf("missing invoice")
	}
	if unilateralRefund == nil {
		return nil, fmt.Errorf("missing callback for unilateral refund")
	}

	var preimageHash []byte

	// TODO: move to decodeInvoice
	if IsBolt12Invoice(invoice) {
		decodedInvoice, err := DecodeBolt12Invoice(invoice)
		if err != nil {
			return nil, fmt.Errorf("failed to decode bolt12 invoice: %v", err)
		}
		preimageHash = decodedInvoice.PaymentHash160
	} else {
		_, hash, err := decodeInvoice(invoice)
		if err != nil {
			return nil, fmt.Errorf("failed to decode invoice: %v", err)
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
		return nil, fmt.Errorf("failed to make submarine swap: %v", err)
	}

	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid claim pubkey: %v", err)
	}

	vhtlcAddress, _, vhtlcOpts, err := h.getVHTLC(
		ctx,
		receiverPubkey,
		nil,
		preimageHash,
		arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralClaim),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralRefund),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != vhtlcAddress {
		return nil, fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	ws := h.boltzSvc.NewWebsocket()
	if err := ws.ConnectAndSubscribe(ctx, []string{swap.Id}, 5*time.Second); err != nil {
		return nil, err
	}

	receivers := []clientTypes.Receiver{{To: swap.Address, Amount: swap.ExpectedAmount}}
	var txid string
	for range 3 {
		// Fund the VHTLC
		txid, err = h.arkClient.SendOffChain(ctx, receivers)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "vtxo_already_spent") {
				continue
			}
			return nil, fmt.Errorf("failed to pay to vHTLC address: %v", err)
		}
		break
	}
	if err != nil {
		log.WithError(err).Error("failed to pay to vHTLC address")
		return nil, fmt.Errorf("something went wrong, please retry")
	}

	swapDetails := &Swap{
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

	contextTimeout := time.Second * time.Duration(h.timeout)
	timeoutCtx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()
	ctx = timeoutCtx

	for {
		select {
		case update, ok := <-ws.Updates:
			if !ok {
				oldWs := ws
				nextWs := h.boltzSvc.NewWebsocket()
				if err := nextWs.ConnectAndSubscribe(
					ctx, []string{swap.Id}, 5*time.Second,
				); err != nil {
					continue
				}
				_ = oldWs.Close()
				ws = nextWs
				continue
			}

			switch boltz.ParseEvent(update.Status) {
			case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
				// Refund the VHTLC if the swap fails
				withReceiver := true
				swapDetails.Status = SwapFailed

				txid, err := h.RefundSwap(
					context.Background(), SwapTypeSubmarine, swap.Id, withReceiver, *vhtlcOpts, nil,
				)
				if err != nil {
					log.WithError(err).Warnf("failed to refund swap %s collaboratively", swap.Id)
					go func() {
						if err := unilateralRefund(*swapDetails); err != nil {
							log.WithError(err).Errorf(
								"failed to refund swap %s unilaterally", swap.Id,
							)
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
				if err := unilateralRefund(*swapDetails); err != nil {
					log.WithError(err).Errorf("failed to refund swap %s unilaterally", swap.Id)
				}
			}()

			return swapDetails, nil
		}
	}

}

func (h *SwapHandler) reverseSwap(
	ctx context.Context, amount uint64, preimage []byte, postProcess func(swap Swap) error,
) (Swap, error) {
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
	invoiceAmount, gotPreimageHash, err := decodeInvoice(swap.Invoice)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode invoice: %v", err)
	}

	if !bytes.Equal(preimageHash, gotPreimageHash) {
		return Swap{}, fmt.Errorf(
			"invalid preimage hash: expected %x, got %x", preimageHash, gotPreimageHash,
		)
	}
	if invoiceAmount != amount {
		return Swap{}, fmt.Errorf(
			"invalid invoice amount: expected %d, got %d", amount, invoiceAmount,
		)
	}

	vhtlcAddress, _, vhtlcOpts, err := h.getVHTLC(
		ctx,
		nil,
		senderPubkey,
		gotPreimageHash,
		arklib.AbsoluteLocktime(swap.TimeoutBlockHeights.RefundLocktime),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralClaim),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralRefund),
		parseLocktime(swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver),
	)

	swapDetails := Swap{
		Id:           swap.Id,
		Invoice:      swap.Invoice,
		PreimageHash: preimageHash,
		TimeoutInfo:  swap.TimeoutBlockHeights,
		Timestamp:    time.Now().Unix(),
		Status:       SwapPending,
		Amount:       swap.OnchainAmount,
		Opts:         vhtlcOpts,
	}
	if err != nil {
		return swapDetails, fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return swapDetails, fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	inv, err := decodepay.Decodepay(swap.Invoice)
	if err != nil {
		return swapDetails, fmt.Errorf("failed to decode invoice: %v", err)
	}

	go func(swapDetails Swap) {
		if reedeemTxId, err := h.waitAndClaim(
			inv.Expiry, swapDetails.Id, preimage, vhtlcOpts,
		); err != nil {
			swapDetails.Status = SwapFailed
			log.WithError(err).Error("failed to claim VHTLC")
		} else {
			swapDetails.RedeemTxid = reedeemTxId
			swapDetails.Status = SwapSuccess
		}

		if err := postProcess(swapDetails); err != nil {
			log.WithError(err).Error("failed to post process swap")
		}
	}(swapDetails)
	return swapDetails, nil
}

func (h *SwapHandler) getVHTLCFunds(
	ctx context.Context, vhtlcs []*vhtlc.VHTLCScript,
) ([]clientTypes.Vtxo, error) {
	scripts := make([]string, 0, len(vhtlcs))
	for _, vHTLC := range vhtlcs {
		tapKey, _, err := vHTLC.TapTree()
		if err != nil {
			return nil, err
		}

		outScript, err := script.P2TRScript(tapKey)
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, hex.EncodeToString(outScript))
	}

	resp, err := h.arkClient.Indexer().GetVtxos(ctx, indexer.WithScripts(scripts))
	if err != nil {
		return nil, err
	}
	return resp.Vtxos, nil
}

func (h *SwapHandler) getVHTLC(
	_ context.Context,
	receiverPubkey, senderPubkey *btcec.PublicKey, preimageHash []byte,
	refundLocktime arklib.AbsoluteLocktime,
	unilateralClaimDelay, unilateralRefundDelay,
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

	opts := vhtlc.Opts{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		Server:                               h.config.SignerPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}

	vHTLC, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	if err != nil {
		return "", nil, nil, err
	}

	encodedAddr, err := vHTLC.Address(h.config.Network.Addr)
	if err != nil {
		return "", nil, nil, err
	}

	return encodedAddr, vHTLC, &opts, nil
}

func (h *SwapHandler) waitAndClaim(
	invoiceExpiry int, swapId string, preimage []byte, vhtlcOpts *vhtlc.Opts,
) (string, error) {
	expiryDuration := time.Duration(invoiceExpiry) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), expiryDuration*2)
	defer cancel()

	ws := h.boltzSvc.NewWebsocket()
	defer ws.Close()

	err := ws.ConnectAndSubscribe(ctx, []string{swapId}, 5*time.Second)
	if err != nil {
		return "", err
	}

	var txid string
	for {
		select {
		case update, ok := <-ws.Updates:
			if !ok {
				oldWs := ws
				nextWs := h.boltzSvc.NewWebsocket()
				if err := nextWs.ConnectAndSubscribe(
					ctx, []string{swapId}, 5*time.Second,
				); err != nil {
					continue
				}
				_ = oldWs.Close()
				ws = nextWs
				continue
			}
			parsedStatus := boltz.ParseEvent(update.Status)

			confirmed := false
			switch parsedStatus {
			case boltz.TransactionMempool:
				confirmed = true
			case boltz.InvoiceFailedToPay, boltz.TransactionFailed, boltz.TransactionLockupFailed:
				return "", fmt.Errorf("failed to receive payment: %s", update.Status)
			}
			if confirmed {
				interval := 200 * time.Millisecond
				log.Debug("claiming VHTLC with preimage...")
				if err := retry(ctx, interval, func(ctx context.Context) (bool, error) {
					var err error
					txid, err = h.ClaimVHTLC(ctx, preimage, *vhtlcOpts, nil)
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
				return txid, nil
			}
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for boltz to detect payment")
		}
	}
}

const (
	SwapTypeSubmarine = "submarine"
	SwapTypeChain     = "chain"
)

func (h *SwapHandler) collaborativeRefund(
	requestRefund func(string, boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error),
	swapId, refundTx, checkpointTx string,
) (*psbt.Packet, *psbt.Packet, error) {
	refundResp, err := requestRefund(swapId, boltz.RefundSwapRequest{
		Transaction: refundTx,
		Checkpoint:  checkpointTx,
	})
	if err != nil {
		return nil, nil, err
	}

	refundPtx, err := psbt.NewFromRawBytes(strings.NewReader(refundResp.Transaction), true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode refund tx signed by boltz: %s", err)
	}

	checkpointPtx, err := psbt.NewFromRawBytes(strings.NewReader(refundResp.Checkpoint), true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode checkpoint tx signed by boltz: %s", err)
	}

	return refundPtx, checkpointPtx, nil
}

// getBatchSessionArgs takes care of preparing the arguments for the batch session to either claim
// or refund a vhtlc.
// NOTE: signerSession is meant to not be nil only if the collaborative refund path is used.
func (h *SwapHandler) getBatchSessionArgs(
	ctx context.Context, vhtlcOpts vhtlc.Opts, outpoint *clientTypes.Outpoint, signerSession *tree.SignerSession,
) (*batchSessionArgs, error) {
	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(vhtlcOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VHTLC script: %w", err)
	}

	vtxo, pending, err := h.selectClaimableVTXO(ctx, vhtlcScript, outpoint)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, fmt.Errorf("vtxo is pending finalization, call FinalizePendingTxs first")
	}

	myAddr, err := h.arkClient.NewOffchainAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get offchain address: %w", err)
	}

	if signerSession == nil {
		ephemeralKey, err := btcec.NewPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to create ephemeral key: %w", err)
		}

		ephemeralSignerSession := tree.NewTreeSignerSession(ephemeralKey)
		signerSession = &ephemeralSignerSession
	}

	vtxoTapscripts := []clientTypes.VtxoWithTapTree{{
		Vtxo:       *vtxo,
		Tapscripts: vhtlcScript.GetRevealedTapscripts(),
	}}

	return &batchSessionArgs{
		vhtlcScript:     vhtlcScript,
		totalAmount:     vtxo.Amount,
		destinationAddr: myAddr,
		signerSession:   *signerSession,
		vtxos:           vtxoTapscripts,
	}, nil
}

func (h *SwapHandler) selectClaimableVTXO(
	ctx context.Context,
	vhtlcScript *vhtlc.VHTLCScript,
	outpoint *clientTypes.Outpoint,
) (*clientTypes.Vtxo, bool, error) {

	vtxos, err := h.getVHTLCFunds(ctx, []*vhtlc.VHTLCScript{vhtlcScript})
	if err != nil {
		return nil, false, err
	}
	if len(vtxos) == 0 {
		return nil, false, ErrorNoVtxosFound
	}

	pendingByOutpoint := make(map[string]bool)
	eligibleVtxos := make([]clientTypes.Vtxo, 0, len(vtxos))

	for _, vtxo := range vtxos {
		key := vtxo.Outpoint.String()

		if vtxo.Spent {
			isPending, err := h.isVtxoPending(ctx, vtxo)
			if err != nil {
				return nil, false, err
			}
			if !isPending {
				continue
			}
			pendingByOutpoint[key] = true
		}

		eligibleVtxos = append(eligibleVtxos, vtxo)
	}

	if len(eligibleVtxos) == 0 {
		return nil, false, ErrorNoVtxosFound
	}

	if outpoint != nil {
		for i := range eligibleVtxos {
			v := &eligibleVtxos[i]
			if v.Txid == outpoint.Txid && v.VOut == outpoint.VOut {
				return v, pendingByOutpoint[v.Outpoint.String()], nil
			}
		}
		return nil, false, fmt.Errorf("outpoint %s not found among VTXOs for this VHTLC", outpoint)
	}

	sort.Slice(eligibleVtxos, func(i, j int) bool {
		a, b := eligibleVtxos[i], eligibleVtxos[j]

		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.Txid != b.Txid {
			return a.Txid < b.Txid
		}
		return a.VOut < b.VOut
	})

	v := &eligibleVtxos[0]
	return v, pendingByOutpoint[v.Outpoint.String()], nil
}

func (h *SwapHandler) isVtxoPending(ctx context.Context, vtxo clientTypes.Vtxo) (bool, error) {
	resp, err := h.arkClient.Indexer().GetVtxos(ctx,
		indexer.WithScripts([]string{vtxo.Script}),
		indexer.WithPendingOnly(),
	)
	if err != nil {
		return false, err
	}

	return len(resp.Vtxos) > 0, nil
}
