package swap

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	log "github.com/sirupsen/logrus"
)

type BaseBatchHandler struct {
	arkClient       arksdk.ArkClient
	transportClient client.TransportClient

	intentId      string
	vtxos         []client.TapscriptsVtxo
	receivers     []types.Receiver
	vhtlcScripts  []*vhtlc.VHTLCScript
	config        types.Config
	signerSession tree.SignerSession

	batchSessionId   string
	batchExpiry      arklib.RelativeLocktime
	countSigningDone int
}

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

func (h *BaseBatchHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	if event.Id == h.batchSessionId {
		log.Debugf("batch completed in commitment tx %s", event.Txid)
	}
	return nil
}

func (h *BaseBatchHandler) OnBatchFailed(
	ctx context.Context, event client.BatchFailedEvent,
) error {
	return fmt.Errorf("batch failed: %s", event.Reason)
}

func (h *BaseBatchHandler) OnTreeTxEvent(
	ctx context.Context, event client.TreeTxEvent,
) error {
	return nil
}

func (h *BaseBatchHandler) OnTreeSignatureEvent(
	ctx context.Context, event client.TreeSignatureEvent,
) error {
	return nil
}

func (h *BaseBatchHandler) OnTreeSigningStarted(
	ctx context.Context, event client.TreeSigningStartedEvent, vtxoTree *tree.TxTree,
) (bool, error) {
	signerPubKey := h.signerSession.GetPublicKey()
	if !slices.Contains(event.CosignersPubkeys, signerPubKey) {
		return true, nil
	}

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

func (h *BaseBatchHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	return false, nil
}

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

type ClaimBatchHandler struct {
	BaseBatchHandler
	preimage []byte
}

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

func (h *ClaimBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("vtxo and connector trees fully signed, building and signing claim forfeits...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	builder := &claimForfeitBuilder{preimage: h.preimage}
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

func (h *BaseBatchHandler) createAndSignForfeits(
	ctx context.Context,
	connectorsLeaves []*psbt.Packet,
	builder forfeitBuilder,
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

		connector, connectorOutpoint, err := extractConnector(connectorTx)
		if err != nil {
			return nil, fmt.Errorf("connector not found for vtxo %s: %w", vtxo.Outpoint.String(), err)
		}

		vtxoScript, err := script.ParseVtxoScript(vtxo.Tapscripts)
		if err != nil {
			return nil, err
		}

		vtxoTapKey, vtxoTapTree, err := vtxoScript.TapTree()
		if err != nil {
			return nil, err
		}

		vhtlcScript := h.vhtlcScripts[i]
		settlementClosure := builder.getSettlementClosure(vhtlcScript)

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

		forfeitClosure := selectForfeitClosure(vhtlcScript, true, false)
		if _, ok := builder.(*refundForfeitBuilder); ok {
			refundBuilder := builder.(*refundForfeitBuilder)
			forfeitClosure = selectForfeitClosure(vhtlcScript, false, refundBuilder.withReceiver)
		}
		vtxoLocktime, vtxoSequence := extractLocktimeAndSequence(forfeitClosure)

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

		if err := builder.buildForfeit(forfeitPtx); err != nil {
			return nil, err
		}

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

func (h *RefundBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("vtxo and connector trees fully signed, building and signing refund forfeits...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	builder := &refundForfeitBuilder{withReceiver: h.withReceiver}
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

type DelegateRefundBatchHandler struct {
	RefundBatchHandler
	partialForfeitTx string
}

func NewDelegateRefundBatchHandler(
	arkClient arksdk.ArkClient,
	transportClient client.TransportClient,
	intentId string,
	vtxos []client.TapscriptsVtxo,
	receivers []types.Receiver,
	withReceiver bool,
	vhtlcScripts []*vhtlc.VHTLCScript,
	config types.Config,
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
		},
		partialForfeitTx: partialForfeitTx,
	}
}

func (h *DelegateRefundBatchHandler) OnBatchFinalization(
	ctx context.Context,
	event client.BatchFinalizationEvent,
	vtxoTree, connectorTree *tree.TxTree,
) error {
	log.Debug("completing delegate refund forfeit...")

	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(h.partialForfeitTx), true)
	if err != nil {
		return fmt.Errorf("failed to decode partial forfeit tx: %w", err)
	}

	updater, err := psbt.NewUpdater(forfeitPtx)
	if err != nil {
		return fmt.Errorf("failed to create PSBT updater: %w", err)
	}

	connectors := connectorTree.Leaves()
	if len(connectors) == 0 {
		return fmt.Errorf("no connectors in tree")
	}
	connector := connectors[0]

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

	if err := updater.AddInSighashType(txscript.SigHashDefault, 1); err != nil {
		return fmt.Errorf("failed to set sighash for connector: %w", err)
	}

	encodedForfeitTx, err := updater.Upsbt.B64Encode()
	if err != nil {
		return fmt.Errorf("failed to encode forfeit tx: %w", err)
	}

	signedForfeitTx, err := h.arkClient.SignTransaction(ctx, encodedForfeitTx)
	if err != nil {
		return fmt.Errorf("failed to sign forfeit: %w", err)
	}

	if err := h.transportClient.SubmitSignedForfeitTxs(ctx, []string{signedForfeitTx}, ""); err != nil {
		return fmt.Errorf("failed to submit signed forfeit: %w", err)
	}

	log.Debug("delegate refund forfeit submitted successfully")
	return nil
}

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

func (h *SwapHandler) buildClaimIntent(
	ctx context.Context,
	session *settlementSession,
	preimage []byte,
) (string, error) {
	vtxoScript, err := script.ParseVtxoScript(session.vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", fmt.Errorf("no forfeit closures found")
	}

	forfeitClosure, err := selectForfeitClosureFromClosures(forfeitClosures, true)
	if err != nil {
		return "", err
	}

	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	claimTapscript, err := session.vhtlcScript.ClaimTapscript()
	if err != nil {
		return "", fmt.Errorf("failed to get claim tapscript for intent: %w", err)
	}

	inputs, tapLeaves, arkFields, err := buildIntentInputs(
		session.vtxos, session.vhtlcScript, claimTapscript, inputSequence,
	)
	if err != nil {
		return "", err
	}

	receivers := []types.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}}

	intentMessage, err := createIntentMessage(session.signerSession)
	if err != nil {
		return "", err
	}

	outputs, err := buildIntentOutputs(receivers)
	if err != nil {
		return "", err
	}

	proof, err := intent.New(intentMessage, inputs, outputs, uint32(vtxoLocktime))
	if err != nil {
		return "", fmt.Errorf("failed to build intent proof: %w", err)
	}

	if err := addForfeitLeafProof(proof, session.vhtlcScript, forfeitClosure); err != nil {
		return "", err
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

func (h *SwapHandler) buildRefundIntent(
	ctx context.Context,
	session *settlementSession,
) (string, error) {
	vtxoScript, err := script.ParseVtxoScript(session.vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", fmt.Errorf("no forfeit closures found")
	}

	forfeitClosure, err := selectForfeitClosureFromClosures(forfeitClosures, false)
	if err != nil {
		return "", err
	}

	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	refundTapscript, err := session.vhtlcScript.RefundTapscript(false)
	if err != nil {
		return "", fmt.Errorf("failed to get refund tapscript for intent: %w", err)
	}

	inputs, tapLeaves, arkFields, err := buildIntentInputs(
		session.vtxos, session.vhtlcScript, refundTapscript, inputSequence,
	)
	if err != nil {
		return "", err
	}

	receivers := []types.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}}

	intentMessage, err := createIntentMessage(session.signerSession)
	if err != nil {
		return "", err
	}

	outputs, err := buildIntentOutputs(receivers)
	if err != nil {
		return "", err
	}

	proof, err := intent.New(intentMessage, inputs, outputs, uint32(vtxoLocktime))
	if err != nil {
		return "", fmt.Errorf("failed to build intent proof: %w", err)
	}

	if err := addForfeitLeafProof(proof, session.vhtlcScript, forfeitClosure); err != nil {
		return "", err
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

func selectForfeitClosure(vhtlcScript *vhtlc.VHTLCScript, isClaimPath, withReceiver bool) script.Closure {
	if isClaimPath {
		// Claim path always uses ConditionMultisigClosure
		return vhtlcScript.ClaimClosure
	}

	if withReceiver {
		return vhtlcScript.RefundClosure
	}
	return vhtlcScript.RefundWithoutReceiverClosure
}

func extractLocktimeAndSequence(closure script.Closure) (arklib.AbsoluteLocktime, uint32) {
	if cltv, ok := closure.(*script.CLTVMultisigClosure); ok {
		return cltv.Locktime, wire.MaxTxInSequenceNum - 1
	}
	return arklib.AbsoluteLocktime(0), wire.MaxTxInSequenceNum
}

func selectForfeitClosureFromClosures(forfeitClosures []script.Closure, isClaimPath bool) (script.Closure, error) {
	if isClaimPath {
		// Claim path: find ConditionMultisigClosure
		for _, fc := range forfeitClosures {
			if _, ok := fc.(*script.ConditionMultisigClosure); ok {
				return fc, nil
			}
		}
		return nil, fmt.Errorf("ConditionMultisigClosure not found for claim path")
	}

	// Refund path: find CLTVMultisigClosure (sweep closure)
	for _, fc := range forfeitClosures {
		if _, ok := fc.(*script.CLTVMultisigClosure); ok {
			return fc, nil
		}
	}
	return nil, fmt.Errorf("CLTVMultisigClosure not found for refund path")
}

func buildIntentInputs(
	vtxos []types.Vtxo,
	vhtlcScript *vhtlc.VHTLCScript,
	settlementTapscript *waddrmgr.Tapscript,
	inputSequence uint32,
) ([]intent.Input, []*arklib.TaprootMerkleProof, [][]*psbt.Unknown, error) {
	vhtlcTapKey, vhtlcTapTree, err := vhtlcScript.TapTree()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get VHTLC tap tree: %w", err)
	}

	inputs := make([]intent.Input, 0, len(vtxos))
	tapLeaves := make([]*arklib.TaprootMerkleProof, 0, len(vtxos))
	arkFields := make([][]*psbt.Unknown, 0, len(vtxos))

	for _, vtxo := range vtxos {
		vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid vtxo txid %s: %w", vtxo.Txid, err)
		}

		settlementTapscriptLeaf := txscript.NewBaseTapLeaf(settlementTapscript.RevealedScript)
		merkleProof, err := vhtlcTapTree.GetTaprootMerkleProof(settlementTapscriptLeaf.TapHash())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get taproot merkle proof: %w", err)
		}

		pkScript, err := script.P2TRScript(vhtlcTapKey)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create P2TR script: %w", err)
		}

		inputs = append(inputs, intent.Input{
			OutPoint: &wire.OutPoint{
				Hash:  *vtxoTxHash,
				Index: vtxo.VOut,
			},
			Sequence: inputSequence,
			WitnessUtxo: &wire.TxOut{
				Value:    int64(vtxo.Amount),
				PkScript: pkScript,
			},
		})

		tapLeaves = append(tapLeaves, merkleProof)
		vhtlcTapscripts := vhtlcScript.GetRevealedTapscripts()
		taptreeField, err := txutils.VtxoTaprootTreeField.Encode(vhtlcTapscripts)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to encode tapscripts: %w", err)
		}
		arkFields = append(arkFields, []*psbt.Unknown{taptreeField})
	}

	return inputs, tapLeaves, arkFields, nil
}

func createIntentMessage(signerSession tree.SignerSession) (string, error) {
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
	return intentMessage, nil
}

func addForfeitLeafProof(
	proof *intent.Proof,
	vhtlcScript *vhtlc.VHTLCScript,
	forfeitClosure script.Closure,
) error {
	_, vhtlcTapTree, err := vhtlcScript.TapTree()
	if err != nil {
		return fmt.Errorf("failed to get VHTLC tap tree: %w", err)
	}

	forfeitScript, err := forfeitClosure.Script()
	if err != nil {
		return fmt.Errorf("failed to get forfeit script: %w", err)
	}

	forfeitLeaf := txscript.NewBaseTapLeaf(forfeitScript)
	leafProof, err := vhtlcTapTree.GetTaprootMerkleProof(forfeitLeaf.TapHash())
	if err != nil {
		return fmt.Errorf("failed to get forfeit merkle proof: %w", err)
	}

	if leafProof != nil {
		proof.Packet.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
			{
				ControlBlock: leafProof.ControlBlock,
				Script:       leafProof.Script,
				LeafVersion:  txscript.BaseLeafVersion,
			},
		}
	}

	return nil
}

type forfeitBuilder interface {
	buildForfeit(forfeitPtx *psbt.Packet) error
	getSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure
}

type claimForfeitBuilder struct {
	preimage []byte
}

func (b *claimForfeitBuilder) buildForfeit(forfeitPtx *psbt.Packet) error {
	if err := txutils.SetArkPsbtField(
		forfeitPtx, 0, txutils.ConditionWitnessField, wire.TxWitness{b.preimage},
	); err != nil {
		return fmt.Errorf("failed to inject preimage: %w", err)
	}
	return nil
}

func (b *claimForfeitBuilder) getSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
	return vhtlcScript.ClaimClosure
}

type refundForfeitBuilder struct {
	withReceiver bool
}

func (b *refundForfeitBuilder) buildForfeit(_ *psbt.Packet) error {
	return nil
}

func (b *refundForfeitBuilder) getSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
	if b.withReceiver {
		return vhtlcScript.RefundClosure
	}
	return vhtlcScript.RefundWithoutReceiverClosure
}

func extractConnector(connectorTx *psbt.Packet) (*wire.TxOut, *wire.OutPoint, error) {
	for outIndex, output := range connectorTx.UnsignedTx.TxOut {
		if bytes.Equal(txutils.ANCHOR_PKSCRIPT, output.PkScript) {
			continue
		}

		return output, &wire.OutPoint{
			Hash:  connectorTx.UnsignedTx.TxHash(),
			Index: uint32(outIndex),
		}, nil
	}

	return nil, nil, fmt.Errorf("connector output not found")
}

func buildForfeitTransaction(
	vtxo client.TapscriptsVtxo,
	vtxoTapKey *btcec.PublicKey,
	settlementTapscript *psbt.TaprootTapLeafScript,
	connector *wire.TxOut,
	connectorOutpoint *wire.OutPoint,
	vtxoLocktime arklib.AbsoluteLocktime,
	vtxoSequence uint32,
	forfeitPkScript []byte,
) (*psbt.Packet, error) {
	vtxoOutputScript, err := script.P2TRScript(vtxoTapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2TR script: %w", err)
	}

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return nil, fmt.Errorf("invalid vtxo txid: %w", err)
	}

	vtxoInput := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoPrevout := &wire.TxOut{
		Value:    int64(vtxo.Amount),
		PkScript: vtxoOutputScript,
	}

	forfeitPtx, err := tree.BuildForfeitTx(
		[]*wire.OutPoint{vtxoInput, connectorOutpoint},
		[]uint32{vtxoSequence, wire.MaxTxInSequenceNum},
		[]*wire.TxOut{vtxoPrevout, connector},
		forfeitPkScript,
		uint32(vtxoLocktime),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build forfeit tx: %w", err)
	}

	forfeitPtx.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{settlementTapscript}

	return forfeitPtx, nil
}

func buildIntentOutputs(receivers []types.Receiver) ([]*wire.TxOut, error) {
	outputs := make([]*wire.TxOut, 0, len(receivers))
	for _, receiver := range receivers {
		decodedAddr, err := arklib.DecodeAddressV0(receiver.To)
		if err != nil {
			return nil, fmt.Errorf("failed to decode receiver address: %w", err)
		}

		pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create receiver pkScript: %w", err)
		}

		outputs = append(outputs, &wire.TxOut{
			Value:    int64(receiver.Amount),
			PkScript: pkScript,
		})
	}
	return outputs, nil
}