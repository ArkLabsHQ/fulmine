package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/go-sdk/client"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

type DelegatorService struct {
	svc *Service
	fee uint64
	
	cachedDelegatorAddress *arklib.Address

	intentsMtx sync.Mutex
	registeredIntents map[string]string // intent hash -> task id
	cancelFunc context.CancelFunc
}

type DelegateInfo struct {
	DelegatorPublicKey string
	Fee uint64
	DelegatorAddress string
}

func NewDelegatorService(svc *Service, fee uint64) *DelegatorService {
	s := &DelegatorService{
		svc: svc,
		fee: fee,
	}

	if err := s.restorePendingTasks(); err != nil {
		log.WithError(err).Warn("failed to restore pending tasks")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel
	go s.listenBatchEvents(ctx)

	return s
}

func (s *DelegatorService) Stop() {
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
}

func (s *DelegatorService) GetDelegateInfo(ctx context.Context) (*DelegateInfo, error) {
	offchainAddr, err := s.getDelegatorAddress(ctx)
	if err != nil {
		return nil, err
	}

	encodedAddr, err := offchainAddr.EncodeV0()
	if err != nil {
		return nil, err
	}

	return &DelegateInfo{
		DelegatorPublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Fee: s.fee,
		DelegatorAddress: encodedAddr,
	}, nil
}

func (s *DelegatorService) Delegate(
	ctx context.Context, 
	intentMessage intent.RegisterMessage, intentProof intent.Proof, forfeit *psbt.Packet,
) error {
	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}
	// TODO : validate intent
	// TODO : validate forfeit

	task, err := s.newDelegateTask(ctx, intentMessage, intentProof, forfeit)
	if err != nil {
		return err
	}

	// validate delegator fee
	if task.Fee < s.fee {
		return fmt.Errorf("delegator fee is less than the required fee (expected at least %d, got %d)", s.fee, task.Fee)
	}

	// TODO validate intent fee

	err = s.svc.dbSvc.Delegate().AddOrUpdate(ctx, *task)
	if err != nil {
		return err
	}

	// schedule task

	return nil
}

func (s *DelegatorService) newDelegateTask(
	ctx context.Context, message intent.RegisterMessage, proof intent.Proof, forfeit *psbt.Packet,
) (*domain.DelegateTask, error) {
	id := uuid.New().String()
	encodedMessage, err := message.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent message: %w", err)
	}
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent proof: %w", err)
	}
	encodedForfeit, err := forfeit.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode forfeit: %w", err)
	}

	delegatorAddr, err := s.getDelegatorAddress(ctx)
	if err != nil {
		return nil, err
	}

	delegatorAddrScript, err := delegatorAddr.GetPkScript()
	if err != nil {
		return nil, err
	}

	feeAmount := int64(0)

	// search for the fee output in intent proof
	for _, output := range proof.UnsignedTx.TxOut {
		if bytes.Equal(output.PkScript, delegatorAddrScript) {
			feeAmount = output.Value
			break
		}
	}

	if message.ValidAt == 0 {
		return nil, fmt.Errorf("invalid valid at")
	}

	ScheduledAt := time.Unix(message.ValidAt, 0)

	inputs := proof.GetOutpoints()
	if len(inputs) != 1 {
		return nil, fmt.Errorf("invalid number of inputs: got %d, expected 1", len(inputs))
	}

	return &domain.DelegateTask{
		ID: id,
		Intent: domain.Intent{
			Message: encodedMessage,
			Proof: encodedProof,
		},
		ForfeitTx: encodedForfeit,
		Fee: uint64(feeAmount),
		DelegatorPublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Input: inputs[0],
		ScheduledAt: ScheduledAt,
		Status: domain.DelegateTaskStatusPending,
	}, nil
}

func (s *DelegatorService) getDelegatorAddress(ctx context.Context) (*arklib.Address, error) {
	if s.cachedDelegatorAddress != nil {
		return s.cachedDelegatorAddress, nil
	}

	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	_, addr, _, _, _, err := s.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}

	decodedAddr, err := arklib.DecodeAddressV0(addr)
	if err != nil {
		return nil, err
	}

	s.cachedDelegatorAddress = decodedAddr

	return decodedAddr, nil
}

func (s *DelegatorService) restorePendingTasks() error {
	ctx := context.Background()
	pendingTasks, err := s.svc.dbSvc.Delegate().GetAllPending(ctx)
	if err != nil {
		return err
	}

	for _, pendingTask := range pendingTasks {
		if err = s.svc.schedulerSvc.ScheduleTaskAtTime(pendingTask.ScheduledAt, func() {
			if err := s.executeDelegate(pendingTask.ID); err != nil {
				log.WithError(err).Warnf("failed to execute delegate task %s", pendingTask.ID)
			}
		}); err != nil {
			log.WithError(err).Warnf("failed to schedule delegate task %s", pendingTask.ID)
			continue
		}
	}

	return nil
}

func (s *DelegatorService) executeDelegate(id string) error {
	ctx := context.Background()
	repo := s.svc.dbSvc.Delegate()
	task, err := repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	intentId, err := s.svc.grpcClient.RegisterIntent(ctx, task.Intent.Proof, task.Intent.Message)
	if err != nil {
		if err := task.Fail(err.Error()); err != nil {
			return err
		}
		if err := repo.AddOrUpdate(ctx, *task); err != nil {
			return err
		}
		return nil
	}

	buf := sha256.Sum256([]byte(intentId))
	hashedIntentId := hex.EncodeToString(buf[:])

	s.intentsMtx.Lock()
	s.registeredIntents[hashedIntentId] = id
	s.intentsMtx.Unlock()

	return nil
}

func (s *DelegatorService) reconnectEventStream(ctx context.Context, currentStop func()) (<-chan client.BatchEventChannel, func(), error) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 5 * time.Minute
		backoffFactor  = 2.0
	)

	// Clean up previous connection if any
	if currentStop != nil {
		currentStop()
	}

	backoff := initialBackoff
	attempt := 0

	for {
		attempt++
		log.WithFields(log.Fields{
			"attempt": attempt,
			"backoff": backoff,
		}).Warn("event stream closed, attempting to reconnect...")

		// Try to connect
		eventsCh, stop, err := s.svc.grpcClient.GetEventStream(ctx, nil)
		if err == nil {
			log.WithField("attempt", attempt).Info("successfully reconnected to event stream")
			return eventsCh, stop, nil
		}

		log.WithError(err).WithField("attempt", attempt).Warn("failed to reconnect to event stream")

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.Info("context cancelled, stopping reconnect attempts")
			return nil, nil, ctx.Err()
		default:
		}

		// Calculate next backoff (exponential increase)
		nextBackoff := time.Duration(float64(backoff) * backoffFactor)
		if nextBackoff > maxBackoff {
			nextBackoff = maxBackoff
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			log.Info("context cancelled during backoff, stopping reconnect attempts")
			return nil, nil, ctx.Err()
		case <-time.After(nextBackoff):
			backoff = nextBackoff
		}
	}
}

func (s *DelegatorService) listenBatchEvents(ctx context.Context) {
	var eventsCh <-chan client.BatchEventChannel
	var stop func()

	var err error
	eventsCh, stop, err = s.svc.grpcClient.GetEventStream(ctx, nil)
	if err != nil {
		log.WithError(err).Error("failed to establish initial connection to event stream")
		return
	}

	flatVtxoTree := make([]tree.TxTreeNode, 0)
	flatConnectorTree := make([]tree.TxTreeNode, 0)
	var vtxoTree, connectorTree *tree.TxTree
	type selectedTask struct {
		id string
		intentHash string
	}
	selectedDelegatorTasks := make([]selectedTask, 0)
	batchExpiry := arklib.RelativeLocktime{
		Type: arklib.LocktimeTypeBlock,
		Value: math.MaxUint32,
	}

	signerSession := tree.NewTreeSignerSession(s.svc.privateKey)

	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case notify, ok := <-eventsCh:
			if !ok {
				flatVtxoTree = make([]tree.TxTreeNode, 0)
				flatConnectorTree = make([]tree.TxTreeNode, 0)
				vtxoTree = nil
				connectorTree = nil
				
				newEventsCh, newStop, err := s.reconnectEventStream(ctx, stop)
				if err != nil {
					log.WithError(err).Error("failed to reconnect to event stream, stopping listenBatchEvents...")
					return
				}
				eventsCh = newEventsCh
				stop = newStop
				continue
			}
			if notify.Err != nil {
				log.WithError(notify.Err).Error("error received from event stream")
				continue
			}

			switch event := notify.Event; event.(type) {
			case client.BatchStartedEvent:
				flatVtxoTree = make([]tree.TxTreeNode, 0)
				flatConnectorTree = make([]tree.TxTreeNode, 0)
				vtxoTree = nil
				connectorTree = nil
				selectedDelegatorTasks = make([]selectedTask, 0)
				signerSession = tree.NewTreeSignerSession(s.svc.privateKey)

				e := event.(client.BatchStartedEvent)
				s.intentsMtx.Lock()
				for _, intentHash := range e.HashedIntentIds {
					if id, ok := s.registeredIntents[intentHash]; ok {
						selectedDelegatorTasks = append(selectedDelegatorTasks, selectedTask{id: id, intentHash: intentHash})
					}
				}
				s.intentsMtx.Unlock()
				batchExpiry = parseLocktime(uint32(e.BatchExpiry))
				continue
			// batch is done, mark tasks as success and remove from registered intents
			case client.BatchFinalizedEvent:
				if len(selectedDelegatorTasks) == 0 {
					continue
				}

				for _, selectedTask := range selectedDelegatorTasks {
					repo := s.svc.dbSvc.Delegate()
					task, err := repo.GetByID(ctx, selectedTask.id)
					if err != nil {
						log.WithError(err).Warnf("failed to get delegate task %s, cannot mark as done", selectedTask.id)
						continue
					}
					if err := task.Success(); err != nil {
						log.WithError(err).Warnf("failed to mark delegate task %s as done", selectedTask.id)
						continue
					}
					if err := repo.AddOrUpdate(ctx, *task); err != nil {
						log.WithError(err).Warnf("failed to update delegate task %s", selectedTask.id)
						continue
					}
					s.intentsMtx.Lock()
					delete(s.registeredIntents, selectedTask.intentHash)
					s.intentsMtx.Unlock()
				}

				selectedDelegatorTasks = make([]selectedTask, 0)
				continue
			// batch failed, try to re-register selected intents
			case client.BatchFailedEvent:
				 for _, selectedTask := range selectedDelegatorTasks {
					if err := s.executeDelegate(selectedTask.id); err != nil {
						log.WithError(err).Warnf("failed to re-register delegate task %s", selectedTask.id)
						continue
					}
				 }
				selectedDelegatorTasks = make([]selectedTask, 0)
				continue
			// we received a tree tx event msg, let's update the vtxo/connector tree.
			case client.TreeTxEvent:
				treeTxEvent := event.(client.TreeTxEvent)

				if treeTxEvent.BatchIndex == 0 {
					flatVtxoTree = append(flatVtxoTree, treeTxEvent.Node)
				} else {
					flatConnectorTree = append(flatConnectorTree, treeTxEvent.Node)
				}

				continue
			case client.TreeSignatureEvent:
				if vtxoTree == nil {
					return
				}

				event := event.(client.TreeSignatureEvent)

				if err := addSignatureToTxTree(event, vtxoTree); err != nil {
					log.WithError(err).Warnf("failed to add signature to vtxo tree")
					continue
				}
				continue
			// the musig2 session started, let's send our nonces.
			case client.TreeSigningStartedEvent:
				var err error
				vtxoTree, err = tree.NewTxTree(flatVtxoTree)
				if err != nil {
					return
				}


				event := event.(client.TreeSigningStartedEvent)

				if err := s.onTreeSigningStarted(ctx, signerSession, batchExpiry, event, vtxoTree); err != nil {
					log.WithError(err).Warnf("failed to handle tree signing started event")
					continue
				}
				continue
			// we received the fully signed vtxo and connector trees, let's send our signed forfeit
			// txs and optionally signed boarding utxos included in the commitment tx.
			case client.TreeNoncesEvent:
				event := event.(client.TreeNoncesEvent)
				_, err := s.onTreeNonces(ctx, event, signerSession)
				if err != nil {
					log.WithError(err).Warnf("failed to handle tree nonces event")
				}
				continue
			case client.BatchFinalizationEvent:
				if len(flatConnectorTree) > 0 {
					var err error
					connectorTree, err = tree.NewTxTree(flatConnectorTree)
					if err != nil {
						continue
					}
				}

				connectorsLeaves := connectorTree.Leaves()

				// TODO : exclude recoverable coins
				selectedTasksIds := make([]string, 0, len(selectedDelegatorTasks))
				for _, selectedTask := range selectedDelegatorTasks {
					selectedTasksIds = append(selectedTasksIds, selectedTask.id)
				}

				if err := s.processForfeits(ctx, connectorsLeaves, selectedTasksIds); err != nil {
					log.WithError(err).Warnf("failed to process forfeits")
					continue
				}
			}
		}
	}
}



func (s *DelegatorService) onTreeSigningStarted(
	ctx context.Context, signerSession tree.SignerSession, batchExpiry arklib.RelativeLocktime, 
	event client.TreeSigningStartedEvent, vtxoTree *tree.TxTree,
) error {
	signerPubKey := signerSession.GetPublicKey()
	if !slices.Contains(event.CosignersPubkeys, signerPubKey) {
		return nil // skip
	}

	cfg, err := s.svc.GetConfigData(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config data: %w", err)
	}

	sweepClosure := script.CSVMultisigClosure{
		MultisigClosure: script.MultisigClosure{PubKeys: []*btcec.PublicKey{cfg.ForfeitPubKey}},
		Locktime:       batchExpiry,
	}

	script, err := sweepClosure.Script()
	if err != nil {
		return fmt.Errorf("failed to get sweep closure script: %w", err)
	}

	commitmentTx, err := psbt.NewFromRawBytes(strings.NewReader(event.UnsignedCommitmentTx), true)
	if err != nil {
		return fmt.Errorf("failed to parse commitment tx: %w", err)
	}

	batchOutput := commitmentTx.UnsignedTx.TxOut[0]
	batchOutputAmount := batchOutput.Value

	sweepTapLeaf := txscript.NewBaseTapLeaf(script)
	sweepTapTree := txscript.AssembleTaprootScriptTree(sweepTapLeaf)
	root := sweepTapTree.RootNode.TapHash()

	if err := signerSession.Init(root.CloneBytes(), batchOutputAmount, vtxoTree); err != nil {
		return err
	}

	nonces, err := signerSession.GetNonces()
	if err != nil {
		return err
	}

	return s.svc.grpcClient.SubmitTreeNonces(ctx, event.Id, signerSession.GetPublicKey(), nonces)
}

func (s *DelegatorService) onTreeNonces(
	ctx context.Context, event client.TreeNoncesEvent,
	signerSession tree.SignerSession,
) (bool, error) {
	hasAllNonces, err := signerSession.AggregateNonces(event.Txid, event.Nonces)
	if err != nil {
		return false, err
	}

	if !hasAllNonces {
		return false, nil
	}

	sigs, err := signerSession.Sign()
	if err != nil {
		return false, err
	}

	if err := s.svc.grpcClient.SubmitTreeSignatures(
		ctx, event.Id, signerSession.GetPublicKey(), sigs,
	); err != nil {
		return false, err
	}

	return true, nil
}

func (s *DelegatorService) processForfeits(
	ctx context.Context, connectorsLeaves []*psbt.Packet, selectedTasksIds []string,
) (error) {
	repo := s.svc.dbSvc.Delegate()
	forfeitTxs := make([]*psbt.Packet, 0, len(selectedTasksIds))
	for _, selectedTaskId := range selectedTasksIds {
		task, err := repo.GetByID(ctx, selectedTaskId)
		if err != nil {
			return fmt.Errorf("failed to get delegate task %s: %w", selectedTaskId, err)
		}

		forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(task.ForfeitTx), true)
		if err != nil {
			return fmt.Errorf("failed to parse forfeit tx: %w", err)
		}
		forfeitTxs = append(forfeitTxs, forfeitPtx)
	}

	if len(forfeitTxs) > len(connectorsLeaves) {
		return fmt.Errorf("insufficient forfeit txs: got %d, need %d", len(forfeitTxs), len(connectorsLeaves))
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
			Sequence: wire.MaxTxInSequenceNum,
		})
		forfeitTx.Inputs[0].SighashType = txscript.SigHashAll

		encodedForfeitTx, err := forfeitTx.B64Encode()
		if err != nil {
			return fmt.Errorf("failed to encode forfeit tx: %w", err)
		}

		signedForfeitTx, err := s.svc.SignTransaction(ctx, encodedForfeitTx)
		if err != nil {
			return fmt.Errorf("failed to sign forfeit: %w", err)
		}

		signedForfeitTxs = append(signedForfeitTxs, signedForfeitTx)
	}

	return s.svc.grpcClient.SubmitSignedForfeitTxs(ctx, signedForfeitTxs, "")
}


