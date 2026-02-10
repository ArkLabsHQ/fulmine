package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/utils"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/go-sdk/client"
	indexer "github.com/arkade-os/go-sdk/indexer"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

// Batch session handler of the delegator service
type delegatorBatchSessionHandler struct {
	utils.Musig2BatchSessionHandler
	delegator     *DelegatorService
	selectedTasks []registeredIntent
}

// BatchStarted event doesn't have to be handled by the delegator session
// it is handled before creating the handler in a dedicated goroutine.
func (h *delegatorBatchSessionHandler) OnBatchStarted(
	context.Context, client.BatchStartedEvent,
) (bool, error) {
	return true, nil
}

// OnBatchFinalized mark the delegates as done and delete the intent from the registered
// intents map
func (h *delegatorBatchSessionHandler) OnBatchFinalized(
	ctx context.Context, event client.BatchFinalizedEvent,
) error {
	repo := h.delegator.svc.dbSvc.Delegate()
	taskIds := make([]string, 0, len(h.selectedTasks))
	for _, selectedTask := range h.selectedTasks {
		taskIds = append(taskIds, selectedTask.taskID)
		h.delegator.intentsMtx.Lock()
		delete(h.delegator.registeredIntents, selectedTask.intentIDHash())
		h.delegator.intentsMtx.Unlock()
	}

	return repo.CompleteTasks(ctx, event.Txid, taskIds...)
}

// OnBatchFailed re-register the delegates that failed to join the batch
func (h *delegatorBatchSessionHandler) OnBatchFailed(
	context.Context, client.BatchFailedEvent,
) error {
	for _, selectedTask := range h.selectedTasks {
		if err := h.delegator.registerDelegate(selectedTask.taskID); err != nil {
			log.WithError(err).Warnf("failed to re-register delegate %s", selectedTask.taskID)
			continue
		}
	}
	log.Warnf("batch failed, %d delegates re-registered", len(h.selectedTasks))
	return fmt.Errorf("batch failed")
}

// OnBatchFinalization submit the delegated forfeit transactions to arkd
func (h *delegatorBatchSessionHandler) OnBatchFinalization(
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

func (h *delegatorBatchSessionHandler) submitForfeitTxs(
	ctx context.Context, connectorsLeaves []*psbt.Packet, selectedTasksIds []string,
) error {
	if len(connectorsLeaves) == 0 {
		return nil
	}
	if len(selectedTasksIds) == 0 {
		return nil
	}

	repo := h.delegator.svc.dbSvc.Delegate()
	forfeitTxs := make([]*psbt.Packet, 0)

	for _, selectedTaskId := range selectedTasksIds {
		task, err := repo.GetByID(ctx, selectedTaskId)
		if err != nil {
			return fmt.Errorf("failed to get delegate %s: %w", selectedTaskId, err)
		}

		// include only the forfeit tx of vtxo that are not recoverable
		outpoints := make([]types.Outpoint, len(task.Intent.Inputs))
		for i, input := range task.Intent.Inputs {
			outpoints[i] = types.Outpoint{
				Txid: input.Hash.String(),
				VOut: input.Index,
			}
		}
		opts := indexer.GetVtxosRequestOption{}
		if err := opts.WithOutpoints(outpoints); err != nil {
			log.WithError(err).Warnf("failed to set outpoints for get vtxos request")
			continue
		}
		vtxos, err := h.delegator.svc.indexerClient.GetVtxos(ctx, opts)
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

		encodedForfeitTx, err := forfeitTx.B64Encode()
		if err != nil {
			return fmt.Errorf("failed to encode forfeit tx: %w", err)
		}

		signedForfeitTx, err := h.delegator.svc.SignTransaction(ctx, encodedForfeitTx)
		if err != nil {
			return fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeitTx)
	}

	return h.delegator.svc.grpcClient.SubmitSignedForfeitTxs(ctx, signedForfeitTxs, "")
}
