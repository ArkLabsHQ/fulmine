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
	log "github.com/sirupsen/logrus"
)

// batchSessionArgs holds the shared state for VHTLC settlement operations.
// This struct encapsulates all the common setup data needed by both claim and refund paths.
type batchSessionArgs struct {
	vhtlcScript     *vhtlc.VHTLCScript
	totalAmount     uint64
	destinationAddr string
	signerSession   tree.SignerSession
	vtxos           []client.TapscriptsVtxo
}

type batchSessionHandler struct {
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

func (h *batchSessionHandler) OnBatchStarted(
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
			h.batchExpiry = parseLocktime(uint32(event.BatchExpiry))
			log.Debugf("batch %s started with our intent %s", event.Id, h.intentId)
			return false, nil
		}
	}
	log.Debug("intent id not found in batch proposal, waiting for next one...")
	return true, nil
}

func (h *batchSessionHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	if event.Id == h.batchSessionId {
		log.Debugf("batch completed in commitment tx %s", event.Txid)
	}
	return nil
}

func (h *batchSessionHandler) OnBatchFailed(
	ctx context.Context, event client.BatchFailedEvent,
) error {
	return fmt.Errorf("batch failed: %s", event.Reason)
}

func (h *batchSessionHandler) OnTreeTxEvent(
	ctx context.Context, event client.TreeTxEvent,
) error {
	return nil
}

func (h *batchSessionHandler) OnTreeSignatureEvent(
	ctx context.Context, event client.TreeSignatureEvent,
) error {
	return nil
}

func (h *batchSessionHandler) OnTreeSigningStarted(
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

func (h *batchSessionHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	if h.signerSession == nil {
		return false, fmt.Errorf("tree signer session not set")
	}

	hasAllNonces, err := h.signerSession.AggregateNonces(event.Txid, event.Nonces)
	if err != nil {
		return false, err
	}

	if !hasAllNonces {
		return false, nil
	}

	sigs, err := h.signerSession.Sign()
	if err != nil {
		return false, err
	}

	if err := h.transportClient.SubmitTreeSignatures(
		ctx, event.Id, h.signerSession.GetPublicKey(), sigs,
	); err != nil {
		return false, err
	}

	return true, nil
}

func (h *batchSessionHandler) OnTreeNoncesAggregated(
	ctx context.Context, event client.TreeNoncesAggregatedEvent,
) (bool, error) {
	return false, nil
}

func (h *batchSessionHandler) createAndSignForfeits(
	ctx context.Context, connectorsLeaves []*psbt.Packet, builder forfeitTxBuilder,
) ([]string, error) {
	parsedForfeitAddr, err := btcutil.DecodeAddress(h.config.ForfeitAddress, nil)
	if err != nil {
		return nil, err
	}

	forfeitPkScript, err := txscript.PayToAddrScript(parsedForfeitAddr)
	if err != nil {
		return nil, err
	}

	if len(connectorsLeaves) < len(h.vtxos) {
		return nil, fmt.Errorf(
			"insufficient connectors: got %d, need %d", len(connectorsLeaves), len(h.vtxos),
		)
	}
	if len(h.vhtlcScripts) < len(h.vtxos) {
		return nil, fmt.Errorf(
			"insufficient vhtlc scripts: got %d, need %d", len(h.vhtlcScripts), len(h.vtxos),
		)
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

		_, vtxoTapTree, err := vtxoScript.TapTree()
		if err != nil {
			return nil, err
		}

		vhtlcScript := h.vhtlcScripts[i]
		signingClosure := builder.getSigningClosure(vhtlcScript)

		signingScript, err := signingClosure.Script()
		if err != nil {
			return nil, err
		}

		signingLeaf := txscript.NewBaseTapLeaf(signingScript)
		proof, err := vtxoTapTree.GetTaprootMerkleProof(signingLeaf.TapHash())
		if err != nil {
			return nil, fmt.Errorf("failed to get taproot merkle proof for settlement: %w", err)
		}

		tapscript := &psbt.TaprootTapLeafScript{
			ControlBlock: proof.ControlBlock,
			Script:       proof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		vtxoLocktime, vtxoSequence := extractLocktimeAndSequence(signingClosure)

		forfeitTx, err := builder.buildTx(
			vtxo, tapscript, connector, connectorOutpoint,
			vtxoLocktime, vtxoSequence, forfeitPkScript,
		)
		if err != nil {
			return nil, err
		}

		signedForfeitTx, err := h.arkClient.SignTransaction(ctx, forfeitTx)
		if err != nil {
			return nil, fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeitTx)
	}

	return signedForfeitTxs, nil
}

// claimBatchSessionHandler handles joining a batch session to claim a vhtlc
type claimBatchSessionHandler struct {
	batchSessionHandler
	preimage []byte
}

func newClaimBatchSessionHandler(
	arkClient arksdk.ArkClient,
	transportClient client.TransportClient,
	intentId string,
	vtxos []client.TapscriptsVtxo,
	receivers []types.Receiver,
	preimage []byte,
	vhtlcScripts []*vhtlc.VHTLCScript,
	config types.Config,
	signerSession tree.SignerSession,
) *claimBatchSessionHandler {
	return &claimBatchSessionHandler{
		batchSessionHandler: batchSessionHandler{
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

func (h *claimBatchSessionHandler) OnBatchFinalization(
	ctx context.Context, event client.BatchFinalizationEvent, vtxoTree, connectorTree *tree.TxTree,
) error {
	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	builder := &claimForfeitTxBuilder{preimage: h.preimage}
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

// refundBatchSessionHandler handles joining a batch session to refund a vhtlc alone, once the
// timelock expired
type refundBatchSessionHandler struct {
	batchSessionHandler
	withReceiver bool
	publicKey    *btcec.PublicKey
}

func newRefundBatchSessionHandler(
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
) *refundBatchSessionHandler {
	return &refundBatchSessionHandler{
		batchSessionHandler: batchSessionHandler{
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

func (h *refundBatchSessionHandler) OnBatchFinalization(
	ctx context.Context, event client.BatchFinalizationEvent, vtxoTree, connectorTree *tree.TxTree,
) error {
	if connectorTree == nil {
		return fmt.Errorf("connector tree is nil")
	}

	builder := &refundForfeitTxBuilder{withReceiver: h.withReceiver}
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

// collabRefundBatchSessionHandler handles joining a batch session to collaboratively refund a
// vhtlc using delegates approach
type collabRefundBatchSessionHandler struct {
	refundBatchSessionHandler
	partialForfeitTx string
}

func newCollabRefundBatchSessionHandler(
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
) *collabRefundBatchSessionHandler {
	return &collabRefundBatchSessionHandler{
		refundBatchSessionHandler: refundBatchSessionHandler{
			batchSessionHandler: batchSessionHandler{
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

func (h *collabRefundBatchSessionHandler) OnBatchFinalization(
	ctx context.Context, event client.BatchFinalizationEvent, vtxoTree, connectorTree *tree.TxTree,
) error {
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

	return nil
}

type forfeitTxBuilder interface {
	buildTx(
		vtxo client.TapscriptsVtxo, signingPath *psbt.TaprootTapLeafScript,
		connector *wire.TxOut, connectorOutpoint *wire.OutPoint,
		vtxoLocktime arklib.AbsoluteLocktime, vtxoSequence uint32,
		forfeitPkScript []byte,
	) (string, error)
	getSigningClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure
}

type claimForfeitTxBuilder struct {
	preimage []byte
}

func (b *claimForfeitTxBuilder) buildTx(
	vtxo client.TapscriptsVtxo, tapscript *psbt.TaprootTapLeafScript,
	connector *wire.TxOut, connectorOutpoint *wire.OutPoint,
	vtxoLocktime arklib.AbsoluteLocktime, vtxoSequence uint32,
	forfeitPkScript []byte,
) (string, error) {
	tx, err := buildForfeitTx(
		vtxo, tapscript, connector, connectorOutpoint,
		vtxoLocktime, vtxoSequence, forfeitPkScript,
	)
	if err != nil {
		return "", err
	}
	if err := txutils.SetArkPsbtField(
		tx, 0, txutils.ConditionWitnessField, wire.TxWitness{b.preimage},
	); err != nil {
		return "", fmt.Errorf("failed to inject preimage: %w", err)
	}

	txStr, err := tx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode forfeit tx: %w", err)
	}
	return txStr, nil
}

func (b *claimForfeitTxBuilder) getSigningClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
	return vhtlcScript.ClaimClosure
}

type refundForfeitTxBuilder struct {
	withReceiver bool
}

func (b *refundForfeitTxBuilder) buildTx(
	vtxo client.TapscriptsVtxo, tapscript *psbt.TaprootTapLeafScript,
	connector *wire.TxOut, connectorOutpoint *wire.OutPoint,
	vtxoLocktime arklib.AbsoluteLocktime, vtxoSequence uint32,
	forfeitPkScript []byte,
) (string, error) {
	tx, err := buildForfeitTx(
		vtxo, tapscript, connector, connectorOutpoint,
		vtxoLocktime, vtxoSequence, forfeitPkScript,
	)
	if err != nil {
		return "", err
	}

	txStr, err := tx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode forfeit tx: %w", err)
	}
	return txStr, nil
}

func (b *refundForfeitTxBuilder) getSigningClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
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

func buildForfeitTx(
	vtxo client.TapscriptsVtxo, signingPath *psbt.TaprootTapLeafScript,
	connector *wire.TxOut, connectorOutpoint *wire.OutPoint,
	vtxoLocktime arklib.AbsoluteLocktime, vtxoSequence uint32,
	outScript []byte,
) (*psbt.Packet, error) {
	vtxoOutputScript, err := hex.DecodeString(vtxo.Script)
	if err != nil {
		return nil, fmt.Errorf("invalid vtxo script: %w", err)
	}

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return nil, fmt.Errorf("invalid vtxo txid: %w", err)
	}

	inputs := []*wire.OutPoint{{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}, connectorOutpoint}
	sequences := []uint32{vtxoSequence, wire.MaxTxInSequenceNum}
	prevouts := []*wire.TxOut{{
		Value:    int64(vtxo.Amount),
		PkScript: vtxoOutputScript,
	}, connector}

	tx, err := tree.BuildForfeitTx(inputs, sequences, prevouts, outScript, uint32(vtxoLocktime))
	if err != nil {
		return nil, fmt.Errorf("failed to build forfeit tx: %w", err)
	}

	tx.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{signingPath}
	return tx, nil
}
