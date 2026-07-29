package application

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	indexer "github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

// Batch session handler of the delegate service
type delegateBatchSessionHandler struct {
	musig2BatchSessionHandler
	delegate      *DelegateService
	selectedTasks []registeredIntent
}

// BatchStarted event doesn't have to be handled by the delegate session
// it is handled before creating the handler in a dedicated goroutine.
func (h *delegateBatchSessionHandler) OnBatchStarted(
	context.Context, client.BatchStartedEvent,
) (bool, error) {
	return true, nil
}

// OnBatchFinalized mark the delegates as done and delete the intent from the registered
// intents map
func (h *delegateBatchSessionHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	repo := h.delegate.svc.dbSvc.Delegate()
	taskIds := make([]string, 0, len(h.selectedTasks))
	for _, selectedTask := range h.selectedTasks {
		taskIds = append(taskIds, selectedTask.taskID)
		h.delegate.intentsMtx.Lock()
		delete(h.delegate.registeredIntents, selectedTask.intentIDHash())
		h.delegate.intentsMtx.Unlock()
	}

	return repo.CompleteTasks(ctx, event.Txid, taskIds...)
}

// OnBatchFailed re-register the delegates that failed to join the batch
func (h *delegateBatchSessionHandler) OnBatchFailed(
	context.Context, client.BatchFailedEvent,
) error {
	for _, selectedTask := range h.selectedTasks {
		if err := h.delegate.registerDelegate(selectedTask.taskID); err != nil {
			log.WithError(err).Warnf("failed to re-register delegate %s", selectedTask.taskID)
			continue
		}
	}
	log.Warnf("batch failed, %d delegates re-registered", len(h.selectedTasks))
	return fmt.Errorf("batch failed")
}

// OnBatchFinalization submit the delegated forfeit transactions to arkd
func (h *delegateBatchSessionHandler) OnBatchFinalization(
	ctx context.Context, event client.BatchFinalizationEvent, vtxoTree, connectorTree *tree.TxTree,
) error {
	selectedTasksIds := make([]string, 0, len(h.selectedTasks))
	for _, selectedTask := range h.selectedTasks {
		selectedTasksIds = append(selectedTasksIds, selectedTask.taskID)
	}

	if err := h.submitForfeitTxs(
		ctx, connectorTree.Leaves(), selectedTasksIds,
	); err != nil {
		log.WithError(err).Warnf("failed to submit forfeit txs")
		return err
	}
	return nil
}

func (h *delegateBatchSessionHandler) submitForfeitTxs(
	ctx context.Context, connectorsLeaves []*psbt.Packet, selectedTasksIds []string,
) error {
	if len(connectorsLeaves) == 0 {
		return nil
	}
	if len(selectedTasksIds) == 0 {
		return nil
	}

	repo := h.delegate.svc.dbSvc.Delegate()
	forfeitTxs := make([]*psbt.Packet, 0)

	for _, selectedTaskId := range selectedTasksIds {
		task, err := repo.GetByID(ctx, selectedTaskId)
		if err != nil {
			return fmt.Errorf("failed to get delegate %s: %w", selectedTaskId, err)
		}

		// include only the forfeit tx of vtxo that are not recoverable
		outpoints := make([]clientTypes.Outpoint, len(task.Intent.Inputs))
		for i, input := range task.Intent.Inputs {
			outpoints[i] = clientTypes.Outpoint{
				Txid: input.Hash.String(),
				VOut: input.Index,
			}
		}

		vtxos, err := h.delegate.svc.Indexer().GetVtxos(ctx, indexer.WithOutpoints(outpoints))
		if err != nil {
			log.WithError(err).Warnf("failed to get vtxos for task %s", selectedTaskId)
			continue
		}

		for _, vtxo := range vtxos.Vtxos {
			if vtxo.IsRecoverable() {
				continue // skip recoverable vtxo
			}

			outpoint, err := wire.NewOutPointFromString(vtxo.Outpoint.String())
			if err != nil {
				log.WithError(err).Warnf(
					"failed to parse outpoint for vtxo %s:%d", vtxo.Txid, vtxo.VOut,
				)
				continue
			}

			forfeitTxStr, ok := task.ForfeitTxs[*outpoint]
			if !ok {
				continue
			}
			forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(forfeitTxStr), true)
			if err != nil {
				return fmt.Errorf("failed to parse forfeit tx: %w", err)
			}
			forfeitTxs = append(forfeitTxs, forfeitPtx)
		}
	}

	if len(forfeitTxs) > len(connectorsLeaves) {
		return fmt.Errorf(
			"insufficient connectors: got %d, need %d",
			len(connectorsLeaves), len(forfeitTxs),
		)
	}

	signedForfeitTxs := make([]string, 0, len(forfeitTxs))
	for i, forfeitTx := range forfeitTxs {
		connectorTx := connectorsLeaves[i]
		connector, connectorOutpoint, err := extractConnector(connectorTx)
		if err != nil {
			return fmt.Errorf("connector not found: %w", err)
		}

		// add the connector to the partially signed forfeit tx
		forfeitTx.Inputs = append(forfeitTx.Inputs, psbt.PInput{
			WitnessUtxo: connector,
		})
		forfeitTx.UnsignedTx.TxIn = append(forfeitTx.UnsignedTx.TxIn, &wire.TxIn{
			PreviousOutPoint: *connectorOutpoint,
			Sequence:         wire.MaxTxInSequenceNum,
		})
		forfeitTx.Inputs[0].SighashType = txscript.SigHashDefault

		if err := signForfeitWithDelegateKey(forfeitTx, h.delegate.svc.privateKey); err != nil {
			return fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTx, err := forfeitTx.B64Encode()
		if err != nil {
			return fmt.Errorf("failed to encode forfeit tx: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeitTx)
	}

	return h.delegate.svc.Client().SubmitSignedForfeitTxs(ctx, signedForfeitTxs, "")
}

// musig2BatchSessionHandler implements the Musig2 methods
type musig2BatchSessionHandler struct {
	SweepClosure    script.CSVMultisigClosure
	SignerSession   tree.SignerSession
	TransportClient client.Client
}

func (h *musig2BatchSessionHandler) OnTreeSigningStarted(
	ctx context.Context, event client.TreeSigningStartedEvent, vtxoTree *tree.TxTree,
) (bool, error) {
	signerPubKey := h.SignerSession.GetPublicKey()
	if !slices.Contains(event.CosignersPubkeys, signerPubKey) {
		return true, nil
	}

	script, err := h.SweepClosure.Script()
	if err != nil {
		return false, fmt.Errorf("failed to get sweep closure script: %w", err)
	}

	commitmentTx, err := psbt.NewFromRawBytes(strings.NewReader(event.UnsignedCommitmentTx), true)
	if err != nil {
		return false, fmt.Errorf("failed to parse commitment tx: %w", err)
	}

	if len(commitmentTx.UnsignedTx.TxOut) == 0 {
		// no tree to sign, skip
		return true, nil
	}

	batchOutput := commitmentTx.UnsignedTx.TxOut[0]
	batchOutputAmount := batchOutput.Value

	sweepTapLeaf := txscript.NewBaseTapLeaf(script)
	sweepTapTree := txscript.AssembleTaprootScriptTree(sweepTapLeaf)
	root := sweepTapTree.RootNode.TapHash()

	if err := h.SignerSession.Init(root.CloneBytes(), batchOutputAmount, vtxoTree); err != nil {
		return false, err
	}

	nonces, err := h.SignerSession.GetNonces()
	if err != nil {
		return false, err
	}

	return false, h.TransportClient.SubmitTreeNonces(ctx, event.Id, h.SignerSession.GetPublicKey(), nonces)
}

func (h *musig2BatchSessionHandler) OnTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
) (bool, error) {
	hasAllNonces, err := h.SignerSession.AggregateNonces(event.Txid, event.Nonces)
	if err != nil {
		return false, err
	}

	if !hasAllNonces {
		return false, nil
	}

	sigs, err := h.SignerSession.Sign()
	if err != nil {
		return false, err
	}

	if err := h.TransportClient.SubmitTreeSignatures(
		ctx, event.Id, h.SignerSession.GetPublicKey(), sigs,
	); err != nil {
		return false, err
	}

	return true, nil
}

func (h *musig2BatchSessionHandler) OnTreeNoncesAggregated(
	ctx context.Context, event client.TreeNoncesAggregatedEvent,
) (bool, error) {
	return false, nil
}

func (h *musig2BatchSessionHandler) OnStreamStartedEvent(
	event client.StreamStartedEvent,
) {
}

// signForfeitWithDelegateKey adds the delegate's signature to every tapscript
// leaf of the forfeit's first input that names the delegate's public key.
//
// It deliberately does not go through Wallet.SignTransaction or
// Identity().SignTransaction, neither of which can do this job:
//
//   - The vtxo being forfeited belongs to the delegator's client, not to us, so
//     the wallet's contract manager cannot resolve its script. Wallet.SignTransaction
//     returns the tx UNSIGNED with a nil error in that case (go-sdk sign.go:39),
//     which arkd then rejects as ForfeitInvalidSignature / "missing 1 signatures".
//   - The delegate key is derived at m/86'/coin'/0' (utils.PrivateKeyFromMnemonic)
//     and advertised via GetDelegateInfo, so clients embed its pubkey in their
//     delegation closures. That key is not addressable by the HD identity's key
//     ids, so no key map could make the identity produce this signature.
//
// Under the old single-key wallet the identity key and the delegate key were the
// same key, which is why passing a nil key map used to work. Making HD the
// default split them apart.
//
// Mirrors the single-key identity's signTapscriptSpend, including its use of the
// input's own SighashType, so the bytes produced are unchanged from before.
func signForfeitWithDelegateKey(forfeitTx *psbt.Packet, prvkey *btcec.PrivateKey) error {
	if prvkey == nil {
		return fmt.Errorf("delegate signer key not loaded")
	}
	if len(forfeitTx.Inputs) == 0 {
		return fmt.Errorf("forfeit tx has no inputs")
	}

	// Every input must carry its own prevout: the sighash commits to all of them.
	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	for i := range forfeitTx.Inputs {
		in := forfeitTx.Inputs[i]
		outpoint := forfeitTx.UnsignedTx.TxIn[i].PreviousOutPoint
		switch {
		case in.WitnessUtxo != nil:
			prevouts[outpoint] = in.WitnessUtxo
		case in.NonWitnessUtxo != nil && int(outpoint.Index) < len(in.NonWitnessUtxo.TxOut):
			prevouts[outpoint] = in.NonWitnessUtxo.TxOut[outpoint.Index]
		default:
			return fmt.Errorf("forfeit input %d: missing prevout", i)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txsighashes := txscript.NewTxSigHashes(forfeitTx.UnsignedTx, prevoutFetcher)

	myPubkey := schnorr.SerializePubKey(prvkey.PubKey())
	input := forfeitTx.Inputs[0]
	signed := false

	for _, leaf := range input.TaprootLeafScript {
		closure, err := script.DecodeClosure(leaf.Script)
		if err != nil {
			continue // unknown leaf, not ours to sign
		}
		if !closureHasPubkey(closure, myPubkey) {
			continue
		}

		leafHash := txscript.NewTapLeaf(leaf.LeafVersion, leaf.Script).TapHash()

		preimage, err := txscript.CalcTapscriptSignaturehash(
			txsighashes, input.SighashType, forfeitTx.UnsignedTx, 0,
			prevoutFetcher, txscript.NewBaseTapLeaf(leaf.Script),
		)
		if err != nil {
			return fmt.Errorf("failed to compute forfeit sighash: %w", err)
		}

		sig, err := schnorr.Sign(prvkey, preimage)
		if err != nil {
			return fmt.Errorf("failed to sign forfeit leaf: %w", err)
		}

		// Append rather than replace: the client's signature is already here, and
		// the closure needs both.
		forfeitTx.Inputs[0].TaprootScriptSpendSig = append(
			forfeitTx.Inputs[0].TaprootScriptSpendSig,
			&psbt.TaprootScriptSpendSig{
				XOnlyPubKey: myPubkey,
				LeafHash:    leafHash.CloneBytes(),
				Signature:   sig.Serialize(),
				SigHash:     input.SighashType,
			},
		)
		signed = true
	}

	// Fail loudly. Returning an unsigned forfeit is what produced the opaque
	// ForfeitInvalidSignature bans on the arkd side.
	if !signed {
		return fmt.Errorf(
			"no tapscript leaf on the forfeit input names the delegate key %x", myPubkey,
		)
	}
	return nil
}

// closureHasPubkey reports whether xonly is one of the closure's signers.
func closureHasPubkey(closure script.Closure, xonly []byte) bool {
	var pubkeys []*btcec.PublicKey
	switch c := closure.(type) {
	case *script.CSVMultisigClosure:
		pubkeys = c.PubKeys
	case *script.MultisigClosure:
		pubkeys = c.PubKeys
	case *script.CLTVMultisigClosure:
		pubkeys = c.PubKeys
	case *script.ConditionMultisigClosure:
		pubkeys = c.PubKeys
	default:
		return false
	}

	for _, key := range pubkeys {
		if bytes.Equal(schnorr.SerializePubKey(key), xonly) {
			return true
		}
	}
	return false
}
