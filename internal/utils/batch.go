package utils

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/go-sdk/client"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
)

// Musig2BatchSessionHandler implements the Musig2 methods
// it is used by both the delegator & pkg/swap/batch_handler.go
type Musig2BatchSessionHandler struct {
	SweepClosure script.CSVMultisigClosure
	SignerSession tree.SignerSession
	TransportClient client.TransportClient
}

func (h *Musig2BatchSessionHandler) OnTreeSigningStarted(
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

func (h *Musig2BatchSessionHandler) OnTreeNonces(
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

func (h *Musig2BatchSessionHandler) OnTreeNoncesAggregated(
	ctx context.Context, event client.TreeNoncesAggregatedEvent,
) (bool, error) {
	return false, nil
}

func (h *Musig2BatchSessionHandler) OnStreamStartedEvent(
	event client.StreamStartedEvent,
) {}