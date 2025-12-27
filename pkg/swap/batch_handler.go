package swap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	log "github.com/sirupsen/logrus"
)

// ClaimBatchHandler handles VHTLC claim settlement (for reverse submarine swaps).
// It implements the full BatchEventsHandler interface including tree signing with musig2.
type ClaimBatchHandler struct {
	arkClient       arksdk.ArkClient
	transportClient client.TransportClient

	intentId      string
	vtxos         []client.TapscriptsVtxo
	receivers     []types.Receiver
	preimage      []byte
	vhtlcScripts  []*vhtlc.VHTLCScript
	config        types.Config
	signerSession tree.SignerSession

	// Batch session state
	batchSessionId   string
	batchExpiry      arklib.RelativeLocktime
	countSigningDone int
}

// NewClaimBatchHandler creates a new claim batch handler.
func NewClaimBatchHandler(
	arkClient arksdk.ArkClient,
	transportClient client.TransportClient,
	intentId string,
	vtxos []client.TapscriptsVtxo,
	receivers []types.Receiver,
	preimage []byte,
	vhtlcScripts []*vhtlc.VHTLCScript,
	config types.Config,
	signerSession tree.SignerSession,
) *ClaimBatchHandler {
	return &ClaimBatchHandler{
		arkClient:        arkClient,
		transportClient:  transportClient,
		intentId:         intentId,
		vtxos:            vtxos,
		receivers:        receivers,
		preimage:         preimage,
		vhtlcScripts:     vhtlcScripts,
		config:           config,
		signerSession:    signerSession,
		batchSessionId:   "",
		countSigningDone: 0,
	}
}

// OnBatchStarted implements BatchEventsHandler.
// Confirms registration when our intent is included in the batch.
func (h *ClaimBatchHandler) OnBatchStarted(
	ctx context.Context, event client.BatchStartedEvent,
) (bool, error) {
	buf := sha256.Sum256([]byte(h.intentId))
	hashedIntentId := hex.EncodeToString(buf[:])

	for _, id := range event.HashedIntentIds {
		if id == hashedIntentId {
			if err := h.transportClient.ConfirmRegistration(ctx, h.intentId); err != nil {
				return false, err
			}
			h.batchSessionId = event.Id
			h.batchExpiry = getBatchExpiryLocktime(uint32(event.BatchExpiry))
			log.Debugf("batch %s started with our intent %s", event.Id, h.intentId)
			return false, nil
		}
	}
	log.Debug("intent id not found in batch proposal, waiting for next one...")
	return true, nil
}

// OnBatchFinalized implements BatchEventsHandler.
func (h *ClaimBatchHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	if event.Id == h.batchSessionId {
		log.Debugf("batch completed in commitment tx %s", event.Txid)
	}
	return nil
}

// OnBatchFailed implements BatchEventsHandler.
func (h *ClaimBatchHandler) OnBatchFailed(
	ctx context.Context, event client.BatchFailedEvent,
) error {
	return fmt.Errorf("batch failed: %s", event.Reason)
}

// OnTreeTxEvent implements BatchEventsHandler.
func (h *ClaimBatchHandler) OnTreeTxEvent(
	ctx context.Context, event client.TreeTxEvent,
) error {
	return nil
}

// OnTreeSignatureEvent implements BatchEventsHandler.
func (h *ClaimBatchHandler) OnTreeSignatureEvent(
	ctx context.Context, event client.TreeSignatureEvent,
) error {
	return nil
}

// OnTreeSigningStarted implements BatchEventsHandler.
// Initializes signer sessions and sends nonces for VTXO tree signing.
func (h *ClaimBatchHandler) OnTreeSigningStarted(
	ctx context.Context, event client.TreeSigningStartedEvent, vtxoTree *tree.TxTree,
) (bool, error) {
	myPubkey := h.signerSession.GetPublicKey()
	if !slices.Contains(event.CosignersPubkeys, myPubkey) {
		return true, nil
	}

	// Build sweep closure for batch expiry
	sweepClosure := script.CSVMultisigClosure{
		MultisigClosure: script.MultisigClosure{PubKeys: []*btcec.PublicKey{h.config.ForfeitPubKey}},
		Locktime:        h.batchExpiry,
	}

	script, err := sweepClosure.Script()
	if err != nil {
		return false, err
	}

	commitmentTx, err := psbt.NewFromRawBytes(strings.NewReader(event.UnsignedCommitmentTx), true)
	if err != nil {
		return false, err
	}

	batchOutput := commitmentTx.UnsignedTx.TxOut[0]
	batchOutputAmount := batchOutput.Value

	sweepTapLeaf := txscript.NewBaseTapLeaf(script)
	sweepTapTree := txscript.AssembleTaprootScriptTree(sweepTapLeaf)
	root := sweepTapTree.RootNode.TapHash()

	generateAndSendNonces := func(session tree.SignerSession) error {
		if err := session.Init(root.CloneBytes(), batchOutputAmount, vtxoTree); err != nil {
			return err
		}

		nonces, err := session.GetNonces()
		if err != nil {
			return err
		}

		return h.transportClient.SubmitTreeNonces(ctx, event.Id, session.GetPublicKey(), nonces)
	}

	if err := generateAndSendNonces(h.signerSession); err != nil {
		return false, err
	}

	return false, nil
}

// OnTreeNonces implements BatchEventsHandler.
// Aggregates nonces and signs the VTXO tree.
func (h *ClaimBatchHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	return false, nil
}

// OnTreeNoncesAggregated implements BatchEventsHandler.
func (h *ClaimBatchHandler) OnTreeNoncesAggregated(
	ctx context.Context, event client.TreeNoncesAggregatedEvent,
) (bool, error) {
	h.signerSession.SetAggregatedNonces(event.Nonces)

	sigs, err := h.signerSession.Sign()
	if err != nil {
		return false, err
	}

	err = h.transportClient.SubmitTreeSignatures(
		ctx,
		event.Id,
		h.signerSession.GetPublicKey(),
		sigs,
	)
	return err == nil, err
}

// OnBatchFinalization implements BatchEventsHandler.
// Builds forfeits with preimage injection for VHTLC claim path.
func (h *ClaimBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("vtxo and connector trees fully signed, building and signing claim forfeits...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	forfeits, err := h.createAndSignClaimForfeits(ctx, connectorTree.Leaves())
	if err != nil {
		return fmt.Errorf("failed to create and sign claim forfeits: %w", err)
	}

	if len(forfeits) > 0 {
		if err := h.transportClient.SubmitSignedForfeitTxs(ctx, forfeits, ""); err != nil {
			return fmt.Errorf("failed to submit signed forfeits: %w", err)
		}
	}

	return nil
}

// createAndSignClaimForfeits builds and signs forfeits for VHTLC claim path.
// The key difference from normal forfeits is that we inject the preimage BEFORE signing.
func (h *ClaimBatchHandler) createAndSignClaimForfeits(
	ctx context.Context, connectorsLeaves []*psbt.Packet,
) ([]string, error) {
	parsedForfeitAddr, err := btcutil.DecodeAddress(h.config.ForfeitAddress, nil)
	if err != nil {
		return nil, err
	}

	forfeitPkScript, err := txscript.PayToAddrScript(parsedForfeitAddr)
	if err != nil {
		return nil, err
	}

	signedForfeitTxs := make([]string, 0, len(h.vtxos))
	for i, vtxo := range h.vtxos {
		connectorTx := connectorsLeaves[i]

		// Extract connector output (skip anchor outputs)
		var connector *wire.TxOut
		var connectorOutpoint *wire.OutPoint
		for outIndex, output := range connectorTx.UnsignedTx.TxOut {
			if bytes.Equal(txutils.ANCHOR_PKSCRIPT, output.PkScript) {
				continue
			}

			connector = output
			connectorOutpoint = &wire.OutPoint{
				Hash:  connectorTx.UnsignedTx.TxHash(),
				Index: uint32(outIndex),
			}
			break
		}

		if connector == nil {
			return nil, fmt.Errorf("connector not found for vtxo %s", vtxo.Outpoint.String())
		}

		// Parse VHTLC script from vtxo tapscripts
		vtxoScript, err := script.ParseVtxoScript(vtxo.Tapscripts)
		if err != nil {
			return nil, err
		}

		vtxoTapKey, vtxoTapTree, err := vtxoScript.TapTree()
		if err != nil {
			return nil, err
		}

		// Get the claim closure from VHTLC script
		vhtlcScript := h.vhtlcScripts[i]
		claimClosure := vhtlcScript.ClaimClosure

		claimScript, err := claimClosure.Script()
		if err != nil {
			return nil, err
		}

		claimLeaf := txscript.NewBaseTapLeaf(claimScript)
		claimProof, err := vtxoTapTree.GetTaprootMerkleProof(claimLeaf.TapHash())
		if err != nil {
			return nil, fmt.Errorf("failed to get taproot merkle proof for claim: %w", err)
		}

		tapscript := &psbt.TaprootTapLeafScript{
			ControlBlock: claimProof.ControlBlock,
			Script:       claimProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		// Build forfeit transaction
		// Convert types.Outpoint to wire.OutPoint
		vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return nil, fmt.Errorf("invalid vtxo txid %s: %w", vtxo.Txid, err)
		}
		vtxoOutpoint := wire.OutPoint{
			Hash:  *vtxoTxHash,
			Index: vtxo.VOut,
		}

		vtxoInput := &wire.TxIn{
			PreviousOutPoint: vtxoOutpoint,
			Sequence:         wire.MaxTxInSequenceNum,
		}

		connectorInput := &wire.TxIn{
			PreviousOutPoint: *connectorOutpoint,
			Sequence:         wire.MaxTxInSequenceNum,
		}

		outputs := []*wire.TxOut{
			{
				Value:    int64(vtxo.Amount) + connector.Value,
				PkScript: forfeitPkScript,
			},
			txutils.AnchorOutput(),
		}

		forfeitTx := &wire.MsgTx{
			Version:  3,
			TxIn:     []*wire.TxIn{vtxoInput, connectorInput},
			TxOut:    outputs,
			LockTime: 0,
		}

		forfeitPtx, err := psbt.NewFromUnsignedTx(forfeitTx)
		if err != nil {
			return nil, err
		}

		// Add witness utxo data
		vtxoPkScript, err := script.P2TRScript(vtxoTapKey)
		if err != nil {
			return nil, err
		}

		forfeitPtx.Inputs[0].WitnessUtxo = &wire.TxOut{
			Value:    int64(vtxo.Amount),
			PkScript: vtxoPkScript,
		}
		forfeitPtx.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapscript}
		forfeitPtx.Inputs[0].TaprootInternalKey = schnorr.SerializePubKey(script.UnspendableKey())
		forfeitPtx.Inputs[1].WitnessUtxo = connector

		// CRITICAL: Inject preimage BEFORE signing
		if err := txutils.SetArkPsbtField(
			forfeitPtx, 0, txutils.ConditionWitnessField, wire.TxWitness{h.preimage},
		); err != nil {
			return nil, fmt.Errorf("failed to inject preimage: %w", err)
		}


		//vtxoTxHash, err = chainhash.NewHashFromStr(vtxo.Txid)
		//if err != nil {
		//	return nil, err
		//}
		//
		//vtxoInput1 := &wire.OutPoint{
		//	Hash:  *vtxoTxHash,
		//	Index: vtxo.VOut,
		//}
		//
		//vtxoSequence := wire.MaxTxInSequenceNum
		//if vtxoLocktime != 0 {
		//	vtxoSequence = wire.MaxTxInSequenceNum - 1
		//}
		//
		//vtxoPrevout := &wire.TxOut{
		//	Value:    int64(vtxo.Amount),
		//	PkScript: vtxoOutputScript,
		//}
		//
		//forfeitTx1, err := tree.BuildForfeitTx(
		//	[]*wire.OutPoint{vtxoInput1, connectorOutpoint},
		//	[]uint32{vtxoSequence, wire.MaxTxInSequenceNum},
		//	[]*wire.TxOut{vtxoOutpoint, connector},
		//	forfeitPkScript,
		//	uint32(vtxoLocktime),
		//)
		//if err != nil {
		//	return nil, err
		//}
		//
		//forfeitTx1.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapscript}

		// Sign the forfeit
		b64, err := forfeitPtx.B64Encode()
		if err != nil {
			return nil, err
		}

		signedForfeit, err := h.arkClient.SignTransaction(ctx, b64)
		if err != nil {
			return nil, fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeit)
	}

	return signedForfeitTxs, nil
}

// RefundBatchHandler handles VHTLC refund settlement (for submarine swaps).
// It implements the full BatchEventsHandler interface including tree signing with musig2.
type RefundBatchHandler struct {
	arkClient       arksdk.ArkClient
	transportClient client.TransportClient

	intentId      string
	vtxos         []client.TapscriptsVtxo
	receivers     []types.Receiver
	withReceiver  bool
	vhtlcScripts  []*vhtlc.VHTLCScript
	config        types.Config
	publicKey     *btcec.PublicKey
	signerSession tree.SignerSession

	// Batch session state
	batchSessionId   string
	batchExpiry      arklib.RelativeLocktime
	countSigningDone int
}

// NewRefundBatchHandler creates a new refund batch handler.
func NewRefundBatchHandler(
	arkClient arksdk.ArkClient,
	transportClient client.TransportClient,
	intentId string,
	vtxos []client.TapscriptsVtxo,
	receivers []types.Receiver,
	withReceiver bool,
	vhtlcScripts []*vhtlc.VHTLCScript,
	config types.Config,
	publicKey *btcec.PublicKey,
	signerSession tree.SignerSession,
) *RefundBatchHandler {
	return &RefundBatchHandler{
		arkClient:        arkClient,
		transportClient:  transportClient,
		intentId:         intentId,
		vtxos:            vtxos,
		receivers:        receivers,
		withReceiver:     withReceiver,
		vhtlcScripts:     vhtlcScripts,
		config:           config,
		publicKey:        publicKey,
		signerSession:    signerSession,
		batchSessionId:   "",
		countSigningDone: 0,
	}
}

// OnBatchStarted implements BatchEventsHandler.
// Confirms registration when our intent is included in the batch.
func (h *RefundBatchHandler) OnBatchStarted(
	ctx context.Context, event client.BatchStartedEvent,
) (bool, error) {
	buf := sha256.Sum256([]byte(h.intentId))
	hashedIntentId := hex.EncodeToString(buf[:])

	for _, id := range event.HashedIntentIds {
		if id == hashedIntentId {
			if err := h.transportClient.ConfirmRegistration(ctx, h.intentId); err != nil {
				return false, err
			}
			h.batchSessionId = event.Id
			h.batchExpiry = getBatchExpiryLocktime(uint32(event.BatchExpiry))
			log.Debugf("batch %s started with our intent %s", event.Id, h.intentId)
			return false, nil
		}
	}
	log.Debug("intent id not found in batch proposal, waiting for next one...")
	return true, nil
}

// OnBatchFinalized implements BatchEventsHandler.
func (h *RefundBatchHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	if event.Id == h.batchSessionId {
		log.Debugf("batch completed in commitment tx %s", event.Txid)
	}
	return nil
}

// OnBatchFailed implements BatchEventsHandler.
func (h *RefundBatchHandler) OnBatchFailed(
	ctx context.Context, event client.BatchFailedEvent,
) error {
	return fmt.Errorf("batch failed: %s", event.Reason)
}

// OnTreeTxEvent implements BatchEventsHandler.
func (h *RefundBatchHandler) OnTreeTxEvent(
	ctx context.Context, event client.TreeTxEvent,
) error {
	return nil
}

// OnTreeSignatureEvent implements BatchEventsHandler.
func (h *RefundBatchHandler) OnTreeSignatureEvent(
	ctx context.Context, event client.TreeSignatureEvent,
) error {
	return nil
}

// OnTreeSigningStarted implements BatchEventsHandler.
// Initializes signer sessions and sends nonces for VTXO tree signing.
func (h *RefundBatchHandler) OnTreeSigningStarted(
	ctx context.Context, event client.TreeSigningStartedEvent, vtxoTree *tree.TxTree,
) (bool, error) {
	myPubkey := h.signerSession.GetPublicKey()
	if !slices.Contains(event.CosignersPubkeys, myPubkey) {
		return true, nil
	}

	// Build sweep closure for batch expiry
	sweepClosure := script.CSVMultisigClosure{
		MultisigClosure: script.MultisigClosure{PubKeys: []*btcec.PublicKey{h.config.ForfeitPubKey}},
		Locktime:        h.batchExpiry,
	}

	script, err := sweepClosure.Script()
	if err != nil {
		return false, err
	}

	commitmentTx, err := psbt.NewFromRawBytes(strings.NewReader(event.UnsignedCommitmentTx), true)
	if err != nil {
		return false, err
	}

	batchOutput := commitmentTx.UnsignedTx.TxOut[0]
	batchOutputAmount := batchOutput.Value

	sweepTapLeaf := txscript.NewBaseTapLeaf(script)
	sweepTapTree := txscript.AssembleTaprootScriptTree(sweepTapLeaf)
	root := sweepTapTree.RootNode.TapHash()

	generateAndSendNonces := func(session tree.SignerSession) error {
		if err := session.Init(root.CloneBytes(), batchOutputAmount, vtxoTree); err != nil {
			return err
		}

		nonces, err := session.GetNonces()
		if err != nil {
			return err
		}

		return h.transportClient.SubmitTreeNonces(ctx, event.Id, session.GetPublicKey(), nonces)
	}

	if err := generateAndSendNonces(h.signerSession); err != nil {
		return false, err
	}

	return false, nil
}

// OnTreeNonces implements BatchEventsHandler.
// Aggregates nonces and signs the VTXO tree.
func (h *RefundBatchHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	return false, nil
}

// OnTreeNoncesAggregated implements BatchEventsHandler.
func (h *RefundBatchHandler) OnTreeNoncesAggregated(
	ctx context.Context, event client.TreeNoncesAggregatedEvent,
) (bool, error) {
	h.signerSession.SetAggregatedNonces(event.Nonces)

	sigs, err := h.signerSession.Sign()
	if err != nil {
		return false, err
	}

	err = h.transportClient.SubmitTreeSignatures(
		ctx,
		event.Id,
		h.signerSession.GetPublicKey(),
		sigs,
	)
	return err == nil, err
}

// OnBatchFinalization implements BatchEventsHandler.
// Builds forfeits with appropriate closure for VHTLC refund path.
func (h *RefundBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("vtxo and connector trees fully signed, building and signing refund forfeits...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	forfeits, err := h.createAndSignRefundForfeits(ctx, connectorTree.Leaves())
	if err != nil {
		return fmt.Errorf("failed to create and sign refund forfeits: %w", err)
	}

	if len(forfeits) > 0 {
		if err := h.transportClient.SubmitSignedForfeitTxs(ctx, forfeits, ""); err != nil {
			return fmt.Errorf("failed to submit signed forfeits: %w", err)
		}
	}

	return nil
}

// createAndSignRefundForfeits builds and signs forfeits for VHTLC refund path.
func (h *RefundBatchHandler) createAndSignRefundForfeits(
	ctx context.Context, connectorsLeaves []*psbt.Packet,
) ([]string, error) {
	parsedForfeitAddr, err := btcutil.DecodeAddress(h.config.ForfeitAddress, nil)
	if err != nil {
		return nil, err
	}

	forfeitPkScript, err := txscript.PayToAddrScript(parsedForfeitAddr)
	if err != nil {
		return nil, err
	}

	signedForfeitTxs := make([]string, 0, len(h.vtxos))
	for i, vtxo := range h.vtxos {
		connectorTx := connectorsLeaves[i]

		// Extract connector output (skip anchor outputs)
		var connector *wire.TxOut
		var connectorOutpoint *wire.OutPoint
		for outIndex, output := range connectorTx.UnsignedTx.TxOut {
			if bytes.Equal(txutils.ANCHOR_PKSCRIPT, output.PkScript) {
				continue
			}

			connector = output
			connectorOutpoint = &wire.OutPoint{
				Hash:  connectorTx.UnsignedTx.TxHash(),
				Index: uint32(outIndex),
			}
			break
		}

		if connector == nil {
			return nil, fmt.Errorf("connector not found for vtxo %s", vtxo.Outpoint.String())
		}

		// Parse VHTLC script from vtxo tapscripts
		vtxoScript, err := script.ParseVtxoScript(vtxo.Tapscripts)
		if err != nil {
			return nil, err
		}

		vtxoTapKey, vtxoTapTree, err := vtxoScript.TapTree()
		if err != nil {
			return nil, err
		}

		// Get the appropriate refund closure from VHTLC script
		vhtlcScript := h.vhtlcScripts[i]
		var refundClosure script.Closure
		if h.withReceiver {
			refundClosure = vhtlcScript.RefundClosure
		} else {
			refundClosure = vhtlcScript.RefundWithoutReceiverClosure
		}

		refundScript, err := refundClosure.Script()
		if err != nil {
			return nil, err
		}

		refundLeaf := txscript.NewBaseTapLeaf(refundScript)
		refundProof, err := vtxoTapTree.GetTaprootMerkleProof(refundLeaf.TapHash())
		if err != nil {
			return nil, fmt.Errorf("failed to get taproot merkle proof for refund: %w", err)
		}

		tapscript := &psbt.TaprootTapLeafScript{
			ControlBlock: refundProof.ControlBlock,
			Script:       refundProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		// Build forfeit transaction
		// Convert types.Outpoint to wire.OutPoint
		vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return nil, fmt.Errorf("invalid vtxo txid %s: %w", vtxo.Txid, err)
		}
		vtxoOutpoint := wire.OutPoint{
			Hash:  *vtxoTxHash,
			Index: vtxo.VOut,
		}

		vtxoInput := &wire.TxIn{
			PreviousOutPoint: vtxoOutpoint,
			Sequence:         wire.MaxTxInSequenceNum,
		}

		connectorInput := &wire.TxIn{
			PreviousOutPoint: *connectorOutpoint,
			Sequence:         wire.MaxTxInSequenceNum,
		}

		outputs := []*wire.TxOut{
			{
				Value:    int64(vtxo.Amount) + connector.Value,
				PkScript: forfeitPkScript,
			},
		}

		forfeitTx := &wire.MsgTx{
			Version:  2,
			TxIn:     []*wire.TxIn{vtxoInput, connectorInput},
			TxOut:    outputs,
			LockTime: 0,
		}

		forfeitPtx, err := psbt.NewFromUnsignedTx(forfeitTx)
		if err != nil {
			return nil, err
		}

		// Add witness utxo data
		rootHash := vtxoTapTree.RootNode.TapHash()
		vtxoTaprootOutputKey := txscript.ComputeTaprootOutputKey(vtxoTapKey, rootHash[:])
		vtxoPkScript, err := txscript.PayToTaprootScript(vtxoTaprootOutputKey)
		if err != nil {
			return nil, err
		}

		forfeitPtx.Inputs[0].WitnessUtxo = &wire.TxOut{
			Value:    int64(vtxo.Amount),
			PkScript: vtxoPkScript,
		}
		forfeitPtx.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapscript}
		forfeitPtx.Inputs[0].TaprootInternalKey = schnorr.SerializePubKey(vtxoTapKey)
		forfeitPtx.Inputs[0].TaprootMerkleRoot = rootHash.CloneBytes()

		forfeitPtx.Inputs[1].WitnessUtxo = connector

		// DEBUG: Verify TaprootLeafScript is set before B64Encode (REFUND PATH)
		log.Debugf("[VHTLC-DEBUG-REFUND] PSBT before B64Encode:")
		for i, input := range forfeitPtx.Inputs {
			log.Debugf("[VHTLC-DEBUG-REFUND]   Input %d: TaprootLeafScript count=%d", i, len(input.TaprootLeafScript))
			if len(input.TaprootLeafScript) > 0 {
				log.Debugf("[VHTLC-DEBUG-REFUND]     Script: %x", input.TaprootLeafScript[0].Script)
				log.Debugf("[VHTLC-DEBUG-REFUND]     ControlBlock len: %d", len(input.TaprootLeafScript[0].ControlBlock))
			}
			log.Debugf("[VHTLC-DEBUG-REFUND]   Input %d: TaprootInternalKey: %x", i, input.TaprootInternalKey)
		}

		// Sign the forfeit (no preimage injection for refund path)
		b64, err := forfeitPtx.B64Encode()
		if err != nil {
			return nil, err
		}

		// DEBUG: Verify TaprootLeafScript survives base64 encoding/decoding (REFUND PATH)
		decodedBytes, decErr := base64.StdEncoding.DecodeString(b64)
		if decErr == nil {
			decoded, decErr := psbt.NewFromRawBytes(bytes.NewReader(decodedBytes), false)
			if decErr == nil && decoded != nil {
				for i, input := range decoded.Inputs {
					log.Debugf("[VHTLC-DEBUG-REFUND] After decode - Input %d: TaprootLeafScript count=%d", i, len(input.TaprootLeafScript))
				}
			}
		}

		signedForfeit, err := h.arkClient.SignTransaction(ctx, b64)
		if err != nil {
			return nil, fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeit)
	}

	return signedForfeitTxs, nil
}

// getBatchExpiryLocktime converts block height to RelativeLocktime
func getBatchExpiryLocktime(batchExpiry uint32) arklib.RelativeLocktime {
	if batchExpiry >= 512 {
		return arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: batchExpiry,
		}
	}
	return arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeBlock,
		Value: batchExpiry,
	}
}

// buildVhtlcIntent creates and registers an intent for VHTLC settlement.
//
// CRITICAL: This function CANNOT use arkClient.RegisterIntent because that method
// expects VTXOs with wallet-controlled tapscripts. VHTLCs have custom 3-party
// tapscripts (ConditionMultisigClosure, CLTVMultisigClosure, etc.) that are NOT
// in the wallet's address set.
//
// Instead, we manually build intent.Input structures with VHTLC tapscripts and
// register directly via transportClient.RegisterIntent.
//
// This follows the pattern from ClaimVHTLC (non-batch) in pkg/swap/swap.go:157-245,
// which correctly builds VHTLC inputs with offchain.BuildTxs.
//
// The function:
// 1. Gets appropriate VHTLC tapscript (claim or refund closure)
// 2. Builds intent.Input manually with VHTLC-specific tapscript
// 3. Creates taproot merkle proof for settlement path
// 4. Builds BIP322 intent proof (exit path signature for ownership)
// 5. Injects preimage (if provided) for claim path validation
// 6. Registers intent via transportClient.RegisterIntent
//
// Parameters:
//   - vtxos: VTXOs at VHTLC address (queried from indexer)
//   - vhtlcScript: Parsed VHTLC script with all closures
//   - claimTapscript: Tapscript for claim or refund path (from vhtlcScript.ClaimTapscript() or RefundTapscript())
//   - destinationAddr: Ark offchain address to send settled funds
//   - totalAmount: Sum of all VTXO amounts
//   - preimage: Preimage for claim path (nil for refund path)
//
// Returns intentID for use in batch session.
func (h *SwapHandler) buildVhtlcIntent(
	ctx context.Context,
	vtxos []types.Vtxo,
	vhtlcScript *vhtlc.VHTLCScript,
	claimTapscript *waddrmgr.Tapscript,
	destinationAddr string,
	totalAmount uint64,
	signerSession tree.SignerSession,
	preimage []byte,
) (intentID string, err error) {
	// Get VHTLC taproot key and tree for script computation
	vhtlcTapKey, vhtlcTapTree, err := vhtlcScript.TapTree()
	if err != nil {
		return "", fmt.Errorf("failed to get VHTLC tap tree: %w", err)
	}

	// Build intent inputs manually with VHTLC tapscripts
	inputs := make([]intent.Input, 0, len(vtxos))
	tapLeaves := make([]*arklib.TaprootMerkleProof, 0, len(vtxos))
	arkFields := make([][]*psbt.Unknown, 0, len(vtxos))

	for _, vtxo := range vtxos {
		vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return "", fmt.Errorf("invalid vtxo txid %s: %w", vtxo.Txid, err)
		}

		// Get merkle proof for settlement path (claim or refund closure)
		claimTapscrptLeaf := txscript.NewBaseTapLeaf(claimTapscript.RevealedScript)
		merkleProof, err := vhtlcTapTree.GetTaprootMerkleProof(claimTapscrptLeaf.TapHash())
		if err != nil {
			return "", fmt.Errorf("failed to get taproot merkle proof: %w", err)
		}

		// Use INTERNAL taproot key directly (NOT tweaked)
		// This matches what arkd stores in vtxo.PubKey
		pkScript, err := script.P2TRScript(vhtlcTapKey)
		if err != nil {
			return "", fmt.Errorf("failed to create P2TR script: %w", err)
		}

		// Create intent.Input with VHTLC witness utxo
		inputs = append(inputs, intent.Input{
			OutPoint: &wire.OutPoint{
				Hash:  *vtxoTxHash,
				Index: vtxo.VOut,
			},
			Sequence: wire.MaxTxInSequenceNum,
			WitnessUtxo: &wire.TxOut{
				Value:    int64(vtxo.Amount),
				PkScript: pkScript,
			},
		})

		// Add merkle proof for signing (exit path, not settlement path)
		// Intent proof uses exit path to prove ownership, not reveal settlement path
		tapLeaves = append(tapLeaves, merkleProof)

		// Encode VHTLC tapscripts in custom PSBT field
		vhtlcTapscripts := vhtlcScript.GetRevealedTapscripts()
		taptreeField, err := txutils.VtxoTaprootTreeField.Encode(vhtlcTapscripts)
		if err != nil {
			return "", fmt.Errorf("failed to encode tapscripts: %w", err)
		}
		arkFields = append(arkFields, []*psbt.Unknown{taptreeField})
	}

	// Build receivers
	receivers := []types.Receiver{
		{
			To:     destinationAddr,
			Amount: totalAmount,
		},
	}

	// Create intent message (RegisterMessage, not generic IntentMessage)
	validAt := time.Now()
	intentMessage, err := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		ExpireAt:            validAt.Add(5 * time.Minute).Unix(),
		ValidAt:             validAt.Unix(),
		CosignersPublicKeys: []string{signerSession.GetPublicKey()},
	}.Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode intent message: %w", err)
	}

	// Build outputs for intent proof
	outputs := make([]*wire.TxOut, 0, len(receivers))
	for _, receiver := range receivers {
		decodedAddr, err := arklib.DecodeAddressV0(receiver.To)
		if err != nil {
			return "", fmt.Errorf("failed to decode receiver address: %w", err)
		}

		pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
		if err != nil {
			return "", fmt.Errorf("failed to create receiver pkScript: %w", err)
		}

		outputs = append(outputs, &wire.TxOut{
			Value:    int64(receiver.Amount),
			PkScript: pkScript,
		})
	}

	// Build intent proof using intent.New() from arkd
	// This creates proper BIP-322 proof with toSpend transaction as first input
	proof, err := intent.New(intentMessage, inputs, outputs)
	if err != nil {
		return "", fmt.Errorf("failed to build intent proof: %w", err)
	}

	vtxoScript, err := script.ParseVtxoScript(vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", fmt.Errorf("no forfeit closures found")
	}

	forfeitClosure := forfeitClosures[0]
	forfeitScript, err := forfeitClosure.Script()
	if err != nil {
		return "", fmt.Errorf("failed to get forfeit script: %w", err)
	}

	forfeitLeaf := txscript.NewBaseTapLeaf(forfeitScript)
	leafProof, err := vhtlcTapTree.GetTaprootMerkleProof(forfeitLeaf.TapHash())
	if err != nil {
		return "", fmt.Errorf("failed to get forfeit merkle proof: %w", err)
	}

	if leafProof != nil {
		proof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
			{
				ControlBlock: leafProof.ControlBlock,
				Script:       leafProof.Script,
				LeafVersion:  txscript.BaseLeafVersion,
			},
		}
	}

	for i := range inputs {
		proof.Inputs[i+1].Unknowns = arkFields[i]
		if tapLeaves[i] != nil {
			proof.Inputs[i+1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
				{
					ControlBlock: tapLeaves[i].ControlBlock,
					Script:       tapLeaves[i].Script,
					LeafVersion:  txscript.BaseLeafVersion,
				},
			}
		}
	}

	if err := txutils.SetArkPsbtField(
		&proof.Packet, 1, txutils.ConditionWitnessField, wire.TxWitness{preimage},
	); err != nil {
		return "", fmt.Errorf("failed to inject preimage into intent proof: %w", err)
	}

	// Sign the proof with user's key
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode proof for signing: %w", err)
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, encodedProof)
	if err != nil {
		return "", fmt.Errorf("failed to sign intent proof: %w", err)
	}

	// Register intent via transport client (NOT arkClient.RegisterIntent!)
	intentID, err = h.transportClient.RegisterIntent(ctx, signedProof, intentMessage)
	if err != nil {
		return "", fmt.Errorf("failed to register VHTLC intent: %w", err)
	}

	return intentID, nil
}
