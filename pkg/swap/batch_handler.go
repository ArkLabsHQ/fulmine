package swap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

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
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	log "github.com/sirupsen/logrus"
)

// BaseBatchHandler contains shared fields and methods for all batch handlers.
// It implements the common BatchEventsHandler methods (8 out of 9) that are identical
// across ClaimBatchHandler and RefundBatchHandler.
type BaseBatchHandler struct {
	arkClient       arksdk.ArkClient
	transportClient client.TransportClient

	intentId      string
	vtxos         []client.TapscriptsVtxo
	receivers     []types.Receiver
	vhtlcScripts  []*vhtlc.VHTLCScript
	config        types.Config
	signerSession tree.SignerSession

	// Batch session state
	batchSessionId   string
	batchExpiry      arklib.RelativeLocktime
	countSigningDone int
}

// OnBatchStarted implements BatchEventsHandler.
// Confirms registration when our intent is included in the batch.
func (h *BaseBatchHandler) OnBatchStarted(
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
func (h *BaseBatchHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	if event.Id == h.batchSessionId {
		log.Debugf("batch completed in commitment tx %s", event.Txid)
	}
	return nil
}

// OnBatchFailed implements BatchEventsHandler.
func (h *BaseBatchHandler) OnBatchFailed(
	ctx context.Context, event client.BatchFailedEvent,
) error {
	return fmt.Errorf("batch failed: %s", event.Reason)
}

// OnTreeTxEvent implements BatchEventsHandler.
func (h *BaseBatchHandler) OnTreeTxEvent(
	ctx context.Context, event client.TreeTxEvent,
) error {
	return nil
}

// OnTreeSignatureEvent implements BatchEventsHandler.
func (h *BaseBatchHandler) OnTreeSignatureEvent(
	ctx context.Context, event client.TreeSignatureEvent,
) error {
	return nil
}

// OnTreeSigningStarted implements BatchEventsHandler.
// Initializes signer sessions and sends nonces for VTXO tree signing.
func (h *BaseBatchHandler) OnTreeSigningStarted(
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
func (h *BaseBatchHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	return false, nil
}

// OnTreeNoncesAggregated implements BatchEventsHandler.
func (h *BaseBatchHandler) OnTreeNoncesAggregated(
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

// ClaimBatchHandler handles VHTLC claim settlement (for reverse submarine swaps).
// It embeds BaseBatchHandler and only implements OnBatchFinalization (claim-specific).
type ClaimBatchHandler struct {
	BaseBatchHandler
	preimage []byte
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
		BaseBatchHandler: BaseBatchHandler{
			arkClient:        arkClient,
			transportClient:  transportClient,
			intentId:         intentId,
			vtxos:            vtxos,
			receivers:        receivers,
			vhtlcScripts:     vhtlcScripts,
			config:           config,
			signerSession:    signerSession,
			batchSessionId:   "",
			countSigningDone: 0,
		},
		preimage: preimage,
	}
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

	builder := &ClaimForfeitBuilder{preimage: h.preimage}
	forfeits, err := h.createAndSignForfeits(ctx, connectorTree.Leaves(), builder)
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

// createAndSignForfeits builds and signs forfeits using the provided ForfeitBuilder strategy.
// This unified method handles both claim and refund paths through the strategy pattern.
func (h *BaseBatchHandler) createAndSignForfeits(
	ctx context.Context,
	connectorsLeaves []*psbt.Packet,
	builder ForfeitBuilder,
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

		// Extract connector output using helper
		connector, connectorOutpoint, err := extractConnector(connectorTx)
		if err != nil {
			return nil, fmt.Errorf("connector not found for vtxo %s: %w", vtxo.Outpoint.String(), err)
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

		// Get settlement closure from strategy (claim or refund)
		vhtlcScript := h.vhtlcScripts[i]
		settlementClosure := builder.GetSettlementClosure(vhtlcScript)

		settlementScript, err := settlementClosure.Script()
		if err != nil {
			return nil, err
		}

		settlementLeaf := txscript.NewBaseTapLeaf(settlementScript)
		settlementProof, err := vtxoTapTree.GetTaprootMerkleProof(settlementLeaf.TapHash())
		if err != nil {
			return nil, fmt.Errorf("failed to get taproot merkle proof for settlement: %w", err)
		}

		tapscript := &psbt.TaprootTapLeafScript{
			ControlBlock: settlementProof.ControlBlock,
			Script:       settlementProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		// Select forfeit closure and extract locktime/sequence using helpers
		forfeitClosure := selectForfeitClosure(vhtlcScript, true, false)
		if _, ok := builder.(*RefundForfeitBuilder); ok {
			refundBuilder := builder.(*RefundForfeitBuilder)
			forfeitClosure = selectForfeitClosure(vhtlcScript, false, refundBuilder.withReceiver)
		}
		vtxoLocktime, vtxoSequence := extractLocktimeAndSequence(forfeitClosure)

		// Build forfeit transaction using helper
		forfeitPtx, err := buildForfeitTransaction(
			vtxo,
			vtxoTapKey,
			tapscript,
			connector,
			connectorOutpoint,
			vtxoLocktime,
			vtxoSequence,
			forfeitPkScript,
		)
		if err != nil {
			return nil, err
		}

		// Apply strategy-specific forfeit modifications (preimage injection for claim, nothing for refund)
		if err := builder.BuildForfeit(forfeitPtx); err != nil {
			return nil, err
		}

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
// It embeds BaseBatchHandler and only implements OnBatchFinalization (refund-specific).
type RefundBatchHandler struct {
	BaseBatchHandler
	withReceiver bool
	publicKey    *btcec.PublicKey
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
		BaseBatchHandler: BaseBatchHandler{
			arkClient:        arkClient,
			transportClient:  transportClient,
			intentId:         intentId,
			vtxos:            vtxos,
			receivers:        receivers,
			vhtlcScripts:     vhtlcScripts,
			config:           config,
			signerSession:    signerSession,
			batchSessionId:   "",
			countSigningDone: 0,
		},
		withReceiver: withReceiver,
		publicKey:    publicKey,
	}
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

	builder := &RefundForfeitBuilder{withReceiver: h.withReceiver}
	forfeits, err := h.createAndSignForfeits(ctx, connectorTree.Leaves(), builder)
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

// DelegateRefundBatchHandler extends RefundBatchHandler for delegate mode.
// In delegate mode, the counterparty creates the intent and partial forfeit,
// and Fulmine acts as delegate to complete the batch session.
type DelegateRefundBatchHandler struct {
	RefundBatchHandler
	partialForfeitTx string // Counterparty's partial forfeit (base64 PSBT)
}

// NewDelegateRefundBatchHandler creates a new delegate refund batch handler.
func NewDelegateRefundBatchHandler(
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
	partialForfeitTx string,
) *DelegateRefundBatchHandler {
	return &DelegateRefundBatchHandler{
		RefundBatchHandler: RefundBatchHandler{
			BaseBatchHandler: BaseBatchHandler{
				arkClient:        arkClient,
				transportClient:  transportClient,
				intentId:         intentId,
				vtxos:            vtxos,
				receivers:        receivers,
				vhtlcScripts:     vhtlcScripts,
				config:           config,
				signerSession:    signerSession,
				batchSessionId:   "",
				countSigningDone: 0,
			},
			withReceiver: withReceiver,
			publicKey:    publicKey,
		},
		partialForfeitTx: partialForfeitTx,
	}
}

// OnBatchFinalization completes the forfeit using counterparty's partial signature.
// The key difference from standard refund is that we:
// 1. Load counterparty's partial forfeit (with SIGHASH_ALL | ANYONECANPAY)
// 2. Add connector input to the forfeit
// 3. Fulmine signs to complete (signs RefundClosure + connector)
// 4. Submit to arkd for server cosigning
func (h *DelegateRefundBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("completing delegate refund forfeit...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	// 1. Load counterparty's partial forfeit
	forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(h.partialForfeitTx), true)
	if err != nil {
		return fmt.Errorf("failed to decode partial forfeit tx: %w", err)
	}

	updater, err := psbt.NewUpdater(forfeitPtx)
	if err != nil {
		return fmt.Errorf("failed to create PSBT updater: %w", err)
	}

	// 2. Add connector input to forfeit tx
	connectors := connectorTree.Leaves()
	if len(connectors) == 0 {
		return fmt.Errorf("no connectors in tree")
	}
	connector := connectors[0]

	// Extract connector output (skip anchor outputs)
	var connectorOut *wire.TxOut
	var connectorIndex uint32
	for outIndex, output := range connector.UnsignedTx.TxOut {
		if bytes.Equal(txutils.ANCHOR_PKSCRIPT, output.PkScript) {
			continue
		}
		connectorOut = output
		connectorIndex = uint32(outIndex)
		break
	}

	if connectorOut == nil {
		return fmt.Errorf("connector output not found")
	}

	// Add connector as input
	updater.Upsbt.UnsignedTx.TxIn = append(updater.Upsbt.UnsignedTx.TxIn, &wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  connector.UnsignedTx.TxHash(),
			Index: connectorIndex,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	updater.Upsbt.Inputs = append(updater.Upsbt.Inputs, psbt.PInput{
		WitnessUtxo: &wire.TxOut{
			Value:    connectorOut.Value,
			PkScript: connectorOut.PkScript,
		},
	})

	// 3. Set SIGHASH_DEFAULT for connector input (input 1)
	if err := updater.AddInSighashType(txscript.SigHashDefault, 1); err != nil {
		return fmt.Errorf("failed to set sighash for connector: %w", err)
	}

	encodedForfeitTx, err := updater.Upsbt.B64Encode()
	if err != nil {
		return fmt.Errorf("failed to encode forfeit tx: %w", err)
	}

	// 4. Fulmine signs forfeit (signs RefundClosure for VTXO input + connector input)
	signedForfeitTx, err := h.arkClient.SignTransaction(ctx, encodedForfeitTx)
	if err != nil {
		return fmt.Errorf("failed to sign forfeit: %w", err)
	}

	// 5. Submit signed forfeit to arkd
	if err := h.transportClient.SubmitSignedForfeitTxs(ctx, []string{signedForfeitTx}, ""); err != nil {
		return fmt.Errorf("failed to submit signed forfeit: %w", err)
	}

	log.Debug("delegate refund forfeit submitted successfully")
	return nil
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

// buildClaimIntent builds and registers an intent for VHTLC claim path settlement.
//
// This function is specialized for the claim path and uses the helpers extracted
// from buildVhtlcIntent to reduce complexity. It:
// 1. Parses VHTLC script and selects ConditionMultisigClosure (claim forfeit)
// 2. Extracts locktime and sequence (should be 0 and 0xFFFFFFFF for claim)
// 3. Builds intent inputs, message, and outputs using helpers
// 4. Creates BIP-322 intent proof with locktime
// 5. Adds forfeit leaf proof
// 6. Injects preimage into proof
// 7. Signs and registers intent
//
// Cyclomatic complexity: ~4 (down from 18 in monolithic buildVhtlcIntent)
func (h *SwapHandler) buildClaimIntent(
	ctx context.Context,
	vtxos []types.Vtxo,
	vhtlcScript *vhtlc.VHTLCScript,
	settlementTapscript *waddrmgr.Tapscript,
	destinationAddr string,
	totalAmount uint64,
	signerSession tree.SignerSession,
	preimage []byte,
) (string, error) {
	// Parse VHTLC script to extract forfeit closures
	vtxoScript, err := script.ParseVtxoScript(vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", fmt.Errorf("no forfeit closures found")
	}

	// Select ConditionMultisigClosure for claim path
	forfeitClosure, err := selectForfeitClosureFromScript(forfeitClosures, true)
	if err != nil {
		return "", err
	}

	// Extract locktime and sequence (claim has no locktime)
	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	// Build intent inputs with helpers
	inputs, tapLeaves, arkFields, err := buildIntentInputs(vtxos, vhtlcScript, settlementTapscript, inputSequence)
	if err != nil {
		return "", err
	}

	// Create receivers
	receivers := []types.Receiver{{To: destinationAddr, Amount: totalAmount}}

	// Create intent message
	intentMessage, err := createIntentMessage(signerSession)
	if err != nil {
		return "", err
	}

	// Build intent outputs
	outputs, err := buildIntentOutputs(receivers)
	if err != nil {
		return "", err
	}

	// Build intent proof with locktime
	proof, err := intent.New(intentMessage, inputs, outputs, uint32(vtxoLocktime))
	if err != nil {
		return "", fmt.Errorf("failed to build intent proof: %w", err)
	}

	// Add forfeit leaf proof
	if err := addForfeitLeafProof(proof, vhtlcScript, forfeitClosure); err != nil {
		return "", err
	}

	// Add tap leaves and ark fields to inputs
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

	// Inject preimage for claim path validation
	if err := txutils.SetArkPsbtField(
		&proof.Packet, 1, txutils.ConditionWitnessField, wire.TxWitness{preimage},
	); err != nil {
		return "", fmt.Errorf("failed to inject preimage into intent proof: %w", err)
	}

	// Sign and register intent
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode proof for signing: %w", err)
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, encodedProof)
	if err != nil {
		return "", fmt.Errorf("failed to sign intent proof: %w", err)
	}

	intentID, err := h.transportClient.RegisterIntent(ctx, signedProof, intentMessage)
	if err != nil {
		return "", fmt.Errorf("failed to register VHTLC claim intent: %w", err)
	}

	return intentID, nil
}

// buildRefundIntent builds and registers an intent for VHTLC refund path settlement.
//
// This function is specialized for the refund path and uses the helpers extracted
// from buildVhtlcIntent to reduce complexity. It:
// 1. Parses VHTLC script and selects CLTVMultisigClosure (refund forfeit)
// 2. Extracts locktime and sequence (CLTV-based, sequence=0xFFFFFFFE)
// 3. Builds intent inputs, message, and outputs using helpers
// 4. Creates BIP-322 intent proof with locktime
// 5. Adds forfeit leaf proof
// 6. Signs and registers intent (no preimage injection)
//
// Cyclomatic complexity: ~4 (down from 18 in monolithic buildVhtlcIntent)
func (h *SwapHandler) buildRefundIntent(
	ctx context.Context,
	vtxos []types.Vtxo,
	vhtlcScript *vhtlc.VHTLCScript,
	settlementTapscript *waddrmgr.Tapscript,
	destinationAddr string,
	totalAmount uint64,
	signerSession tree.SignerSession,
) (string, error) {
	// Parse VHTLC script to extract forfeit closures
	vtxoScript, err := script.ParseVtxoScript(vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", fmt.Errorf("no forfeit closures found")
	}

	// Select CLTVMultisigClosure for refund path
	forfeitClosure, err := selectForfeitClosureFromScript(forfeitClosures, false)
	if err != nil {
		return "", err
	}

	// Extract locktime and sequence (refund has CLTV locktime)
	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	// Build intent inputs with helpers
	inputs, tapLeaves, arkFields, err := buildIntentInputs(vtxos, vhtlcScript, settlementTapscript, inputSequence)
	if err != nil {
		return "", err
	}

	// Create receivers
	receivers := []types.Receiver{{To: destinationAddr, Amount: totalAmount}}

	// Create intent message
	intentMessage, err := createIntentMessage(signerSession)
	if err != nil {
		return "", err
	}

	// Build intent outputs
	outputs, err := buildIntentOutputs(receivers)
	if err != nil {
		return "", err
	}

	// Build intent proof with locktime
	proof, err := intent.New(intentMessage, inputs, outputs, uint32(vtxoLocktime))
	if err != nil {
		return "", fmt.Errorf("failed to build intent proof: %w", err)
	}

	// Add forfeit leaf proof
	if err := addForfeitLeafProof(proof, vhtlcScript, forfeitClosure); err != nil {
		return "", err
	}

	// Add tap leaves and ark fields to inputs
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

	// Sign and register intent (no preimage for refund path)
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode proof for signing: %w", err)
	}

	signedProof, err := h.arkClient.SignTransaction(ctx, encodedProof)
	if err != nil {
		return "", fmt.Errorf("failed to sign intent proof: %w", err)
	}

	intentID, err := h.transportClient.RegisterIntent(ctx, signedProof, intentMessage)
	if err != nil {
		return "", fmt.Errorf("failed to register VHTLC refund intent: %w", err)
	}

	return intentID, nil
}
