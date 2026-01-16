package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/go-sdk/client"
	indexer "github.com/arkade-os/go-sdk/indexer"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
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
	delegatorAddrMtx sync.Mutex

	intentsMtx sync.Mutex
	registeredIntents map[string]registeredIntent // intent hash -> task id
	ctx context.Context
	cancelFunc context.CancelFunc

	pendingTasksMtx sync.Mutex
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
		registeredIntents: make(map[string]registeredIntent),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelFunc = cancel

	go func(ctx context.Context) {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
					continue
				}
				if s.svc.publicKey == nil || s.svc.privateKey == nil {
					continue
				}
				s.start()
				log.Debug("delegator background handler started")
				return
			}
		}
	}(ctx)

	return s
}

func (s *DelegatorService) start() {
	if err := s.restorePendingTasks(); err != nil {
		log.WithError(err).Warn("failed to restore pending tasks")
	}

	go s.listenBatchStartedEvents(s.ctx)
}

func (s *DelegatorService) Stop() {
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
}

// GetDelegateInfo returns the data needed to create the intent & forfeit tx for a delegate task.
func (s *DelegatorService) GetDelegateInfo(ctx context.Context) (*DelegateInfo, error) {
	if s.svc.publicKey == nil {
		return nil, fmt.Errorf("service not ready")
	}

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

// Delegate registers a delegate task and schedules it for execution.
// it validates:
// * the intent is not a collaborative exit
// * the intent has at least 1 input and forfeit has matching number of inputs spending Alice + Delegator tapscript.
// * the forfeit amount is correct
// * the forfeit signatures are valid and use sighash type ANYONECANPAY | ALL
// * the intent inputs = forfeit inputs
// * the forfeit sigs are related to tapleaf script
// * the forfeit scripts are multisig closures with 3 pubkeys: Alice, Delegator and Signer
// * there is no pending task with any overlapping input in DB
// once validated, the task is saved to DB and scheduled for execution.
func (s *DelegatorService) Delegate(
	ctx context.Context, 
	intentMessage intent.RegisterMessage, intentProof intent.Proof, forfeits []*psbt.Packet,
) error {
	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}
	
	// TODO validate intent fee

	// reject collaborative exit
	if len(intentMessage.OnchainOutputIndexes) > 0 {
		return fmt.Errorf("delegated collaborative exit is not supported")
	}

	cfg, err := s.svc.GetConfigData(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config data: %w", err)
	}
	addr, err := btcutil.DecodeAddress(cfg.ForfeitAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to decode forfeit address: %w", err)
	}

	forfeitOutputScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return fmt.Errorf("failed to create forfeit output script: %w", err)
	}

	for _, forfeit := range forfeits {
		if len(forfeit.UnsignedTx.TxOut) != 2 {
			return fmt.Errorf("invalid number of forfeit outputs: got %d, expected 2", len(forfeit.UnsignedTx.TxOut))
		}

		if len(forfeit.UnsignedTx.TxIn) != 1 {
			return fmt.Errorf("invalid number of inputs: got %d, expected 1", len(forfeit.UnsignedTx.TxIn))
		}

		// verify forfeit outputs

		// the first should be the forfeit server address
		if !bytes.Equal(forfeit.UnsignedTx.TxOut[0].PkScript, forfeitOutputScript) {
			return fmt.Errorf(
				"wrong forfeit output script, expected %x, got %x", 
				forfeitOutputScript, forfeit.UnsignedTx.TxOut[0].PkScript,
			)
		}

		// the second one should be P2A
		if !bytes.Equal(forfeit.UnsignedTx.TxOut[1].PkScript, txutils.ANCHOR_PKSCRIPT) {
			return fmt.Errorf(
				"wrong anchor output script, expected %x, got %x", 
				txutils.ANCHOR_PKSCRIPT, forfeit.UnsignedTx.TxOut[1].PkScript,
			)
		}

		// validate each forfeit input
		totalForfeitAmount := int64(0)
		if forfeit.Inputs[0].WitnessUtxo == nil {
			return fmt.Errorf("forfeit input witness utxo is nil")
		}

		if len(forfeit.Inputs[0].TaprootScriptSpendSig) != 1 {
			return fmt.Errorf("forfeit input has no taproot script spend sig")
		}

		if len(forfeit.Inputs[0].TaprootLeafScript) != 1 {
			return fmt.Errorf("forfeit input has no taproot leaf script")
		}

		totalForfeitAmount += forfeit.Inputs[0].WitnessUtxo.Value

		// verify forfeit amount (sum of all inputs + dust)
		expectedAmount := int64(cfg.Dust) + totalForfeitAmount
		if expectedAmount != forfeit.UnsignedTx.TxOut[0].Value {
			return fmt.Errorf(
				"wrong forfeit amount, expected %d, got %d", 
				expectedAmount,
				forfeit.UnsignedTx.TxOut[0].Value,
			)
		}

		delegatorXonlyKey := schnorr.SerializePubKey(s.svc.publicKey)
		expectedSigHashType := txscript.SigHashAnyOneCanPay | txscript.SigHashAll

		prevoutMap := make(map[wire.OutPoint]*wire.TxOut)
		forfeitOutpoint := forfeit.UnsignedTx.TxIn[0].PreviousOutPoint
		forfeitSig := forfeit.Inputs[0].TaprootScriptSpendSig[0]
		forfeitLeafScript := forfeit.Inputs[0].TaprootLeafScript[0]

		// verify forfeit sig is related to tapleaf script
		tapLeaf := txscript.NewBaseTapLeaf(forfeitLeafScript.Script)
		tapLeafHash := tapLeaf.TapHash()
		if !bytes.Equal(forfeitSig.LeafHash, tapLeafHash[:]) {
			return fmt.Errorf("forfeit input: missing tapleaf script %x", tapLeafHash)
		}

		// verify the forfeit script 
		var multisigClosure script.MultisigClosure
		valid, err := multisigClosure.Decode(forfeitLeafScript.Script); 
		if err != nil {
			return fmt.Errorf("forfeit input: failed to decode multisig closure: %w", err)
		}
		if !valid {
			return fmt.Errorf("forfeit input: invalid taproot leaf script, expected multisig closure, got %x", forfeitLeafScript.Script)
		}
		if len(multisigClosure.PubKeys) != 3 {
			return fmt.Errorf("forfeit input: invalid multisig closure, expected 3 pubkeys, got %d", len(multisigClosure.PubKeys))
		}

		var delegatorFound, signerFound bool
		for _, pubkey := range multisigClosure.PubKeys {
			xonlyKey := schnorr.SerializePubKey(pubkey)
			if bytes.Equal(xonlyKey, delegatorXonlyKey) {
				delegatorFound = true
				continue
			}

			if bytes.Equal(xonlyKey, forfeitSig.XOnlyPubKey) {
				signerFound = true
			}
		}
		if !delegatorFound {
			return fmt.Errorf("forfeit input: delegator public key not found in taproot leaf script")
		}
		if !signerFound {
			return fmt.Errorf("forfeit input: signer public key not found in taproot leaf script")
		}

		// verify the signature (must be valid and use sighash type ANYONECANPAY | ALL)
		if forfeitSig.SigHash != expectedSigHashType {
			return fmt.Errorf("forfeit input: invalid sighash type, expected AnyoneCanPay | All, got %d", forfeitSig.SigHash)
		}

		prevoutMap[forfeitOutpoint] = forfeit.Inputs[0].WitnessUtxo

		// verify all signatures
		prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevoutMap)

		message, err := txscript.CalcTapscriptSignaturehash(
			txscript.NewTxSigHashes(forfeit.UnsignedTx, prevoutFetcher),
			expectedSigHashType, forfeit.UnsignedTx, 0, prevoutFetcher, tapLeaf,
		)
		if err != nil {
			return fmt.Errorf("forfeit input: failed to calculate sighash: %w", err)
		}

		signerPublicKey, err := schnorr.ParsePubKey(forfeitSig.XOnlyPubKey)
		if err != nil {
			return fmt.Errorf("forfeit input: failed to parse signer public key: %w", err)
		}

		sig, err := schnorr.ParseSignature(forfeitSig.Signature)
		if err != nil {
			return fmt.Errorf("forfeit input: failed to parse signature: %w", err)
		}

		if !sig.Verify(message, signerPublicKey) {
			return fmt.Errorf("forfeit input: invalid forfeit signature")
		}
	}

	task, err := s.newDelegateTask(ctx, intentMessage, intentProof, forfeits)
	if err != nil {
		return err
	}

	// validate delegator fee
	if task.Fee < s.fee {
		return fmt.Errorf("delegator fee is less than the required fee (expected at least %d, got %d)", s.fee, task.Fee)
	}

	// verify forfeit input are referenced in the intent
	for input := range task.ForfeitTxs {
		if !slices.Contains(task.Inputs, input) {
			return fmt.Errorf("forfeit input %s:%d is not referenced in the intent", input.Hash.String(), input.Index)
		}
	}

	// verify all inputs are real VTXOs and not unrolled or spent
	outpoints := make([]types.Outpoint, len(task.Inputs))
	for i, input := range task.Inputs {
		outpoints[i] = types.Outpoint{
			Txid: input.Hash.String(),
			VOut: input.Index,
		}
	}

	opts := indexer.GetVtxosRequestOption{}
	if err := opts.WithOutpoints(outpoints); err != nil {
		return fmt.Errorf("failed to get vtxos: %w", err)
	}
	vtxos, err := s.svc.indexerClient.GetVtxos(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get vtxos: %w", err)
	}
	if len(vtxos.Vtxos) != len(task.Inputs) {
		return fmt.Errorf("expected %d vtxos, got %d", len(task.Inputs), len(vtxos.Vtxos))
	}

	for i, vtxo := range vtxos.Vtxos {
		if vtxo.Spent {
			return fmt.Errorf("input %d is already spent", i)
		}
		if vtxo.Unrolled {
			return fmt.Errorf("input %d is unrolled", i)
		}
	}

	// lock to avoid a new task with overlapping inputs is created while we are adding it to database
	s.pendingTasksMtx.Lock()
	defer s.pendingTasksMtx.Unlock()

	repo := s.svc.dbSvc.Delegate()

	// before saving to database, verify that there is no pending task with any overlapping input
	pendingTaskIDs, err := repo.GetPendingTaskIDsByInputs(ctx, task.Inputs)
	if err != nil {
		return err
	}
	if len(pendingTaskIDs) > 0 {
		// cancel pending tasks with overlapping inputs
		if err := repo.CancelTasks(ctx, pendingTaskIDs...); err != nil {
			return fmt.Errorf("failed to cancel pending tasks: %w", err)
		}
	}

	// save task to database
	if err := repo.Add(ctx, *task); err != nil {
		return err
	}

	// schedule task
	if err := s.svc.schedulerSvc.ScheduleTaskAtTime(task.ScheduledAt, func() {
		if err := s.registerDelegate(task.ID); err != nil {
			log.WithError(err).Warnf("failed to execute delegate task %s", task.ID)
		}
	}); err != nil {
		return fmt.Errorf("failed to schedule delegate task: %w", err)
	}

	return nil
}

func (s *DelegatorService) newDelegateTask(
	ctx context.Context, message intent.RegisterMessage, proof intent.Proof, forfeits []*psbt.Packet,
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

	scheduledAt := time.Unix(message.ValidAt, 0)

	inputs := proof.GetOutpoints()
	if len(inputs) == 0 {
		return nil, fmt.Errorf("invalid number of inputs: got %d, expected at least 1", len(inputs))
	}

	forfeitTxs := make(map[wire.OutPoint]string)
	for _, forfeit := range forfeits {
		if len(forfeit.UnsignedTx.TxIn) != 1 {
			return nil, fmt.Errorf("invalid number of inputs: got %d, expected 1", len(forfeit.UnsignedTx.TxIn))
		}

		encodedForfeit, err := forfeit.B64Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode forfeit: %w", err)
		}
		forfeitTxs[forfeit.UnsignedTx.TxIn[0].PreviousOutPoint] = encodedForfeit
	}

	return &domain.DelegateTask{
		ID: id,
		Intent: domain.Intent{
			Message: encodedMessage,
			Proof: encodedProof,
		},
		ForfeitTxs: forfeitTxs,
		Fee: uint64(feeAmount),
		DelegatorPublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Inputs: inputs,
		ScheduledAt: scheduledAt,
		Status: domain.DelegateTaskStatusPending,
	}, nil
}

func (s *DelegatorService) getDelegatorAddress(ctx context.Context) (*arklib.Address, error) {
	s.delegatorAddrMtx.Lock()
	if s.cachedDelegatorAddress != nil {
		addr := s.cachedDelegatorAddress
		s.delegatorAddrMtx.Unlock()
		return addr, nil
	}
	s.delegatorAddrMtx.Unlock()

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

	s.delegatorAddrMtx.Lock()
	// Double-check: another goroutine might have set it while we were fetching
	if s.cachedDelegatorAddress == nil {
		s.cachedDelegatorAddress = decodedAddr
	} else {
		// Use the cached value if it was set by another goroutine
		decodedAddr = s.cachedDelegatorAddress
	}
	s.delegatorAddrMtx.Unlock()

	return decodedAddr, nil
}

// restorePendingTasks restores all pending tasks from DB and schedules them for execution.
func (s *DelegatorService) restorePendingTasks() error {
	pendingTasks, err := s.svc.dbSvc.Delegate().GetAllPending(s.ctx)
	if err != nil {
		return err
	}

	for _, pendingTask := range pendingTasks {
		select {
		case <-s.ctx.Done(): // context cancelled, stop restoring pending tasks
			return nil
		default:
		}

		taskID := pendingTask.ID // capture value
		if err = s.svc.schedulerSvc.ScheduleTaskAtTime(pendingTask.ScheduledAt, func() {
			if err := s.registerDelegate(taskID); err != nil {
				log.WithError(err).Warnf("failed to execute delegate task %s", taskID)
			}
		}); err != nil {
			log.WithError(err).Warnf("failed to schedule delegate task %s", taskID)
			continue
		}
	}

	return nil
}

func (s *DelegatorService) registerDelegate(id string) error {
	repo := s.svc.dbSvc.Delegate()
	s.pendingTasksMtx.Lock()
	task, err := repo.GetByID(s.ctx, id)
	if err != nil {
		s.pendingTasksMtx.Unlock()
		return err
	}
	s.pendingTasksMtx.Unlock()
	if task.Status != domain.DelegateTaskStatusPending {
		// task is not pending, it has been cancelled by another task
		return nil
	}

	intentId, err := s.svc.grpcClient.RegisterIntent(s.ctx, task.Intent.Proof, task.Intent.Message)
	if err != nil {
		log.WithError(err).Errorf("failed to register intent for delegate task %s", id)
		return repo.FailTasks(s.ctx, err.Error(), id)
	}

	registeredIntent := registeredIntent{
		taskID: id,
		intentID: intentId,
		inputs: task.Inputs,
	}

	s.intentsMtx.Lock()
	s.registeredIntents[registeredIntent.intentIDHash()] = registeredIntent
	s.intentsMtx.Unlock()

	log.Debugf("delegate task %s registered", id)

	return nil
}

// listenBatchStartedEvents check all BatchStartedEvent sent by Ark server and join batch if any include on of the delegated intent.
func (s *DelegatorService) listenBatchStartedEvents(ctx context.Context) {
	log.Debug("listening for batch events")
	var eventsCh <-chan client.BatchEventChannel
	var stop func()
	var err error

	eventsCh, stop, err = s.connectorEventStreamWithRetry(ctx, nil)
	if err != nil {
		log.WithError(err).Error("failed to establish initial connection to event stream")
		return
	}

	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case notify, ok := <-eventsCh:
			if !ok {
				newEventsCh, newStop, err := s.connectorEventStreamWithRetry(ctx, stop)
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

			event, ok := notify.Event.(client.BatchStartedEvent)
			if !ok {
				continue
			}
			selectedTasks := make([]registeredIntent, 0)

			s.intentsMtx.Lock()
			for _, intentHash := range event.HashedIntentIds {
				if registeredIntent, ok := s.registeredIntents[intentHash]; ok {
					selectedTasks = append(selectedTasks, registeredIntent)
				}
			}
			s.intentsMtx.Unlock()
			batchExpiry := parseLocktime(uint32(event.BatchExpiry))

			if len(selectedTasks) == 0 {
				continue
			}

			go s.runDelegatorBatch(ctx, batchExpiry, selectedTasks)
			log.Infof("batch started, selected %d delegate tasks", len(selectedTasks))
		}
	}
}

func (s *DelegatorService) runDelegatorBatch(
	ctx context.Context, batchExpiry arklib.RelativeLocktime, selectedTasks []registeredIntent,
) {
	commitmentTxId, err := s.joinDelegatorBatch(ctx, batchExpiry, selectedTasks)
	if err != nil {
		log.WithError(err).Warnf("failed to join batch")
		return
	}
	countVtxos := 0
	for _, selectedTask := range selectedTasks {
		countVtxos += len(selectedTask.inputs)
	}
	log.WithField("countTasks", len(selectedTasks)).WithField("countVtxos", countVtxos).Infof("batch %s completed", commitmentTxId)
}

// joinDelegatorBatch is launched after the BatchStartedEvent is received and is reponsible to sign vtxo tree and submit forfeits txs.
func (s *DelegatorService) joinDelegatorBatch(
	ctx context.Context, batchExpiry arklib.RelativeLocktime, selectedDelegatorTasks []registeredIntent,
) (string, error) {
	flatVtxoTree := make([]tree.TxTreeNode, 0)
	flatConnectorTree := make([]tree.TxTreeNode, 0)
	var vtxoTree, connectorTree *tree.TxTree

	signerSession := tree.NewTreeSignerSession(s.svc.privateKey)

	topics := make([]string, 0, len(selectedDelegatorTasks)*2 + 1)
	topics = append(topics, hex.EncodeToString(s.svc.publicKey.SerializeCompressed()))
	
	// confirm registrations and compute topics
	for _, selectedTask := range selectedDelegatorTasks {
		if err := s.svc.grpcClient.ConfirmRegistration(ctx, selectedTask.intentID); err != nil {
			log.WithError(err).Warnf("failed to confirm registration for intent %s", selectedTask.intentID)
			continue
		}
		for _, input := range selectedTask.inputs {
			topics = append(topics, types.Outpoint{
				Txid: input.Hash.String(),
				VOut: input.Index,
			}.String())
		}
	}

	eventsCh, stop, err := s.svc.grpcClient.GetEventStream(ctx, topics)
	if err != nil {
		return "", fmt.Errorf(
			"failed to establish initial connection to event stream with event stream topics: %w", 
			err,
		)
	}
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case event, ok := <-eventsCh:
			if !ok {
				return "", fmt.Errorf("event stream closed")
			}
			switch event := event.Event.(type) {
			case client.BatchFinalizedEvent:
				repo := s.svc.dbSvc.Delegate()
				taskIds := make([]string, 0, len(selectedDelegatorTasks))
				for _, selectedTask := range selectedDelegatorTasks {
					taskIds = append(taskIds, selectedTask.taskID)
					s.intentsMtx.Lock()
					delete(s.registeredIntents, selectedTask.intentIDHash())
					s.intentsMtx.Unlock()
				}
				if err := repo.SuccessTasks(ctx, taskIds...); err != nil {
					log.WithError(err).Warnf("failed to mark delegate tasks as done")
					continue
				}
				log.Debugf(
					"batch %s finalized, %d delegate tasks marked as done", 
					event.Txid, len(selectedDelegatorTasks),
				)
				return event.Txid, nil
			// batch failed, try to re-register selected intents
			case client.BatchFailedEvent:
				 for _, selectedTask := range selectedDelegatorTasks {
					if err := s.registerDelegate(selectedTask.taskID); err != nil {
						log.WithError(err).Warnf("failed to re-register delegate task %s", selectedTask.taskID)
						continue
					}
				 }
				 log.Warnf("batch failed, %d delegate tasks re-registered", len(selectedDelegatorTasks))
				return "", fmt.Errorf("batch failed")
			// we received a tree tx event msg, let's update the vtxo/connector tree.
			case client.TreeTxEvent:
				if event.BatchIndex == 0 {
					flatVtxoTree = append(flatVtxoTree, event.Node)
				} else {
					flatConnectorTree = append(flatConnectorTree, event.Node)
				}
			
				continue
			case client.TreeSignatureEvent:
				if vtxoTree == nil {
					return "", fmt.Errorf("vtxo tree is nil")
				}
			
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
					log.WithError(err).Warnf("failed to create vtxo tree")
					continue
				}
			
				if err := s.onTreeSigningStarted(ctx, signerSession, batchExpiry, event, vtxoTree); err != nil {
					log.WithError(err).Warnf("failed to handle tree signing started event")
					continue
				}
				continue
			// we received the fully signed vtxo and connector trees, let's send our signed forfeit
			// txs and optionally signed boarding utxos included in the commitment tx.
			case client.TreeNoncesEvent:
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

				if connectorTree == nil {
					continue
				}
			
				connectorsLeaves := connectorTree.Leaves()
			
				selectedTasksIds := make([]string, 0, len(selectedDelegatorTasks))
				for _, selectedTask := range selectedDelegatorTasks {
					// check if any input is recoverable - if all are recoverable, skip this task
					outpoints := make([]types.Outpoint, len(selectedTask.inputs))
					for i, input := range selectedTask.inputs {
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
					vtxos, err := s.svc.indexerClient.GetVtxos(ctx, opts)
					if err == nil && len(vtxos.Vtxos) > 0 {
						allRecoverable := true
						for _, vtxo := range vtxos.Vtxos {
							if !vtxo.IsRecoverable() {
								allRecoverable = false
								break
							}
						}
						if allRecoverable {
							continue // exclude tasks where all inputs are recoverable, they do not need forfeits
						}
					}

					selectedTasksIds = append(selectedTasksIds, selectedTask.taskID)
				}

				if err := s.submitForfeitTransactions(ctx, connectorsLeaves, selectedTasksIds); err != nil {
					log.WithError(err).Warnf("failed to submit forfeits")
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

func (s *DelegatorService) submitForfeitTransactions(
	ctx context.Context, connectorsLeaves []*psbt.Packet, selectedTasksIds []string,
) error {
	repo := s.svc.dbSvc.Delegate()
	forfeitTxs := make([]*psbt.Packet, 0)
	for _, selectedTaskId := range selectedTasksIds {
		task, err := repo.GetByID(ctx, selectedTaskId)
		if err != nil {
			return fmt.Errorf("failed to get delegate task %s: %w", selectedTaskId, err)
		}

		// Get forfeit transactions for inputs that have them (forfeit transactions are optional)
		for _, input := range task.Inputs {
			forfeitTxStr, ok := task.ForfeitTxs[input]
			if !ok {
				// Skip inputs without forfeit transactions
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
			Sequence: wire.MaxTxInSequenceNum,
		})
		forfeitTx.Inputs[0].SighashType = txscript.SigHashDefault

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

func (s *DelegatorService) connectorEventStreamWithRetry(
	ctx context.Context, currentStop func(),
) (<-chan client.BatchEventChannel, func(), error) {
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