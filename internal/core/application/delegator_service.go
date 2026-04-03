package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	indexer "github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

type DelegateService struct {
	svc *Service
	fee uint64

	cachedDelegateAddress *arklib.Address
	delegateAddrMtx       sync.Mutex

	intentsMtx        sync.Mutex
	registeredIntents map[string]registeredIntent // intent hash -> task id

	ctx        context.Context
	cancelFunc context.CancelFunc

	// delegateMtx is used to prevent concurrent access to delegate tasks
	// while we are monitoring spent vtxos and registering new tasks
	delegateMtx sync.Mutex
}

type DelegateInfo struct {
	PubKey  string
	Fee     uint64
	Address string
}

func newDelegateService(svc *Service, fee uint64) *DelegateService {
	return &DelegateService{
		svc:               svc,
		fee:               fee,
		registeredIntents: make(map[string]registeredIntent),
		delegateAddrMtx:   sync.Mutex{},
		intentsMtx:        sync.Mutex{},
		delegateMtx:       sync.Mutex{},
	}
}

func (s *DelegateService) start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelFunc = cancel
	if err := s.restorePendingTasks(); err != nil {
		log.WithError(err).Warn("failed to restore pending tasks")
	}

	go s.listenBatchStartedEvents(s.ctx)
	go s.monitorVtxosSpent(s.ctx)
}

func (s *DelegateService) Stop() {
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
}

// GetInfo returns the data needed to create the intent & forfeit tx for a delegate task.
func (s *DelegateService) GetInfo(ctx context.Context) (*DelegateInfo, error) {
	if s.svc.publicKey == nil {
		return nil, fmt.Errorf("service not ready")
	}

	offchainAddr, err := s.getDelegateAddress(ctx)
	if err != nil {
		return nil, err
	}

	encodedAddr, err := offchainAddr.EncodeV0()
	if err != nil {
		return nil, err
	}

	return &DelegateInfo{
		PubKey:  hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Fee:     s.fee,
		Address: encodedAddr,
	}, nil
}

// Delegate creates a delegate task, then schedules it for execution if valid
func (s *DelegateService) Delegate(
	ctx context.Context,
	intentMessage intent.RegisterMessage, intentProof intent.Proof, forfeitTxs []*psbt.Packet,
	allowReplace bool,
) error {
	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	task, err := s.newDelegateTask(ctx, intentMessage, intentProof, forfeitTxs)
	if err != nil {
		return err
	}

	repo := s.svc.dbSvc.Delegate()

	// check if we already have a pending task with the same intent txid
	pendingTask, _ := repo.GetPendingTaskByIntentTxID(ctx, task.Intent.Txid)
	if pendingTask != nil {
		// duplicate task, no need to do anything
		return nil
	}

	// lock to avoid a new task with overlapping inputs is created while we are adding it to
	// database
	s.delegateMtx.Lock()
	defer s.delegateMtx.Unlock()

	// before saving to database, verify that there is no pending task with any overlapping input
	pendingTaskIDs, err := repo.GetPendingTaskIDsByInputs(ctx, task.Intent.Inputs)
	if err != nil {
		return err
	}
	if len(pendingTaskIDs) > 0 {
		if !allowReplace {
			return fmt.Errorf("there is a pending task with overlapping inputs")
		}

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
		if err := repo.FailTasks(ctx, err.Error(), task.ID); err != nil {
			log.WithError(err).Warnf("failed to mark delegate task %s as failed", task.ID)
		}

		return fmt.Errorf("failed to schedule delegate task: %w", err)
	}

	return nil
}

func (s *DelegateService) newDelegateTask(
	ctx context.Context,
	message intent.RegisterMessage, proof intent.Proof, forfeitTxs []*psbt.Packet,
) (*domain.DelegateTask, error) {
	cfg, err := s.svc.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}

	if err := validateForfeits(s.svc.publicKey, cfg, forfeitTxs); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	encodedMessage, err := message.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent message: %w", err)
	}
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent proof: %w", err)
	}

	delegateAddr, err := s.getDelegateAddress(ctx)
	if err != nil {
		return nil, err
	}

	delegateAddrScript, err := delegateAddr.GetPkScript()
	if err != nil {
		return nil, err
	}

	feeAmount := int64(0)

	// search for the fee output in intent proof
	for _, output := range proof.UnsignedTx.TxOut {
		if bytes.Equal(output.PkScript, delegateAddrScript) {
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
		return nil, fmt.Errorf(
			"invalid number of inputs: got %d, expected at least 1", len(inputs),
		)
	}

	indexedForfeitTxs := make(map[wire.OutPoint]string)
	for _, forfeit := range forfeitTxs {
		if len(forfeit.UnsignedTx.TxIn) != 1 {
			return nil, fmt.Errorf(
				"invalid number of inputs: got %d, expected 1", len(forfeit.UnsignedTx.TxIn),
			)
		}

		encodedForfeit, err := forfeit.B64Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode forfeit: %w", err)
		}
		indexedForfeitTxs[forfeit.UnsignedTx.TxIn[0].PreviousOutPoint] = encodedForfeit
	}

	task := &domain.DelegateTask{
		ID: id,
		Intent: domain.Intent{
			Message: encodedMessage,
			Proof:   encodedProof,
			Txid:    proof.UnsignedTx.TxID(),
			Inputs:  inputs,
		},
		ForfeitTxs:        indexedForfeitTxs,
		Fee:               uint64(feeAmount),
		DelegatePublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		ScheduledAt:       scheduledAt,
		Status:            domain.DelegateTaskStatusPending,
	}

	// validate delegate fee
	if task.Fee < s.fee {
		return nil, fmt.Errorf(
			"delegate fee is less than the required fee (expected at least %d, got %d)",
			s.fee, task.Fee,
		)
	}

	// verify forfeit input are referenced in the intent
	for input := range task.ForfeitTxs {
		if !slices.Contains(task.Intent.Inputs, input) {
			return nil, fmt.Errorf(
				"forfeit tx input %s:%d is not referenced in the intent",
				input.Hash.String(), input.Index,
			)
		}
	}

	// verify all inputs are real VTXOs and not unrolled or spent
	outpoints := make([]clientTypes.Outpoint, len(task.Intent.Inputs))
	for i, input := range task.Intent.Inputs {
		outpoints[i] = clientTypes.Outpoint{
			Txid: input.Hash.String(),
			VOut: input.Index,
		}
	}

	vtxos, err := s.svc.Indexer().GetVtxos(ctx, indexer.WithOutpoints(outpoints))
	if err != nil {
		return nil, fmt.Errorf("failed to get vtxos: %w", err)
	}
	if len(vtxos.Vtxos) != len(task.Intent.Inputs) {
		return nil, fmt.Errorf(
			"expected %d vtxos, got %d", len(task.Intent.Inputs), len(vtxos.Vtxos),
		)
	}

	for i, vtxo := range vtxos.Vtxos {
		if vtxo.Spent {
			return nil, fmt.Errorf("input %d is already spent", i)
		}
		if vtxo.Unrolled {
			return nil, fmt.Errorf("input %d is unrolled", i)
		}
	}
	return task, nil
}

func (s *DelegateService) getDelegateAddress(ctx context.Context) (*arklib.Address, error) {
	s.delegateAddrMtx.Lock()
	if s.cachedDelegateAddress != nil {
		addr := s.cachedDelegateAddress
		s.delegateAddrMtx.Unlock()
		return addr, nil
	}
	s.delegateAddrMtx.Unlock()

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

	s.delegateAddrMtx.Lock()
	// Double-check: another goroutine might have set it while we were fetching
	if s.cachedDelegateAddress == nil {
		s.cachedDelegateAddress = decodedAddr
	} else {
		// Use the cached value if it was set by another goroutine
		decodedAddr = s.cachedDelegateAddress
	}
	s.delegateAddrMtx.Unlock()

	return decodedAddr, nil
}

// restorePendingTasks restores all pending tasks from DB and schedules them for execution.
func (s *DelegateService) restorePendingTasks() error {
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

func (s *DelegateService) registerDelegate(id string) error {
	repo := s.svc.dbSvc.Delegate()
	s.delegateMtx.Lock()
	task, err := repo.GetByID(s.ctx, id)
	if err != nil {
		s.delegateMtx.Unlock()
		return err
	}
	s.delegateMtx.Unlock()
	if task.Status != domain.DelegateTaskStatusPending {
		// task is not pending, it has been cancelled by another task
		return nil
	}

	intentId, err := s.svc.Client().RegisterIntent(s.ctx, task.Intent.Proof, task.Intent.Message)
	if err != nil {
		log.WithError(err).Errorf("failed to register intent for delegate task %s", id)
		return repo.FailTasks(s.ctx, err.Error(), id)
	}

	registeredIntent := registeredIntent{
		taskID:   id,
		intentID: intentId,
		inputs:   task.Intent.Inputs,
	}

	s.intentsMtx.Lock()
	s.registeredIntents[registeredIntent.intentIDHash()] = registeredIntent
	s.intentsMtx.Unlock()

	log.Debugf("delegate task %s registered", id)

	return nil
}

// listenBatchStartedEvents check all BatchStartedEvent sent by Ark server and join batch if any
// include on of the delegated intent.
func (s *DelegateService) listenBatchStartedEvents(ctx context.Context) {
	log.Debug("listening for batch events")
	var eventsCh <-chan client.BatchEventChannel
	var stop func()
	var err error

	eventsCh, stop, err = s.svc.Client().GetEventStream(ctx, nil)
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
				log.Debug("stream closed")
				return
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

			go s.runDelegateBatch(ctx, batchExpiry, selectedTasks)
			log.Infof("batch started, selected %d delegate tasks", len(selectedTasks))
		}
	}
}

func (s *DelegateService) runDelegateBatch(
	ctx context.Context, batchExpiry arklib.RelativeLocktime, selectedTasks []registeredIntent,
) {
	commitmentTxId, err := s.joinDelegateBatch(ctx, batchExpiry, selectedTasks)
	if err != nil {
		log.WithError(err).Warnf("failed to join batch")
		return
	}
	countVtxos := 0
	for _, selectedTask := range selectedTasks {
		countVtxos += len(selectedTask.inputs)
	}
	log.WithFields(log.Fields{
		"countTasks": len(selectedTasks),
		"countVtxos": countVtxos,
	}).Infof("batch %s completed", commitmentTxId)
}

// joinDelegateBatch is launched after the BatchStartedEvent is received and is reponsible to sign
// vtxo tree and submit forfeits txs.
func (s *DelegateService) joinDelegateBatch(
	ctx context.Context,
	batchExpiry arklib.RelativeLocktime, selectedDelegateTasks []registeredIntent,
) (string, error) {
	flatVtxoTree := make([]tree.TxTreeNode, 0)
	flatConnectorTree := make([]tree.TxTreeNode, 0)
	var vtxoTree, connectorTree *tree.TxTree

	signerSession := tree.NewTreeSignerSession(s.svc.privateKey)

	topics := make([]string, 0, len(selectedDelegateTasks)*2+1)
	topics = append(topics, hex.EncodeToString(s.svc.publicKey.SerializeCompressed()))

	// confirm registrations and compute topics
	for _, selectedTask := range selectedDelegateTasks {
		if err := s.svc.Client().ConfirmRegistration(ctx, selectedTask.intentID); err != nil {
			log.WithError(err).Warnf(
				"failed to confirm registration for intent %s", selectedTask.intentID,
			)
			continue
		}
		for _, input := range selectedTask.inputs {
			topics = append(topics, clientTypes.Outpoint{
				Txid: input.Hash.String(),
				VOut: input.Index,
			}.String())
		}
	}

	eventsCh, stop, err := s.svc.Client().GetEventStream(ctx, topics)
	if err != nil {
		return "", fmt.Errorf(
			"failed to establish initial connection to event stream with event stream topics: %w",
			err,
		)
	}
	defer stop()

	cfg, err := s.svc.GetConfigData(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get config data: %w", err)
	}

	handler := &delegateBatchSessionHandler{
		musig2BatchSessionHandler: musig2BatchSessionHandler{
			SignerSession:   signerSession,
			TransportClient: s.svc.Client(),
			SweepClosure: script.CSVMultisigClosure{
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{cfg.ForfeitPubKey},
				},
				Locktime: batchExpiry,
			},
		},
		delegate:      s,
		selectedTasks: selectedDelegateTasks,
	}

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
				if err := handler.OnBatchFinalized(ctx, event); err != nil {
					log.WithError(err).Warnf("failed to handle batch finalized event")
					continue
				}
				return event.Txid, nil
			case client.BatchFailedEvent:
				return "", handler.OnBatchFailed(ctx, event)
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

				if err := signVtxoTree(event, vtxoTree); err != nil {
					log.WithError(err).Warnf("failed to add signature to vtxo tree")
					continue
				}
				continue
			case client.TreeSigningStartedEvent:
				var err error
				vtxoTree, err = tree.NewTxTree(flatVtxoTree)
				if err != nil {
					log.WithError(err).Warnf("failed to create vtxo tree")
					continue
				}

				if _, err := handler.OnTreeSigningStarted(ctx, event, vtxoTree); err != nil {
					log.WithError(err).Warnf("failed to handle tree signing started event")
					continue
				}
				continue
			case client.TreeNoncesEvent:
				if _, err := handler.OnTreeNonces(ctx, event); err != nil {
					log.WithError(err).Warnf("failed to handle tree nonces event")
				}
			case client.BatchFinalizationEvent:
				if len(flatConnectorTree) == 0 {
					continue
				}
				connectorTree, err = tree.NewTxTree(flatConnectorTree)
				if err != nil {
					log.WithError(err).Warnf("failed to create connector tree")
					continue
				}

				if err := handler.OnBatchFinalization(
					ctx, event, vtxoTree, connectorTree,
				); err != nil {
					log.WithError(err).Warnf("failed to handle batch finalization event")
				}
			}
		}
	}
}

func (s *DelegateService) monitorVtxosSpent(ctx context.Context) {
	const tickerInterval = 5 * time.Second
	log.Debug("monitoring delegated spent vtxos")

	var eventsCh <-chan client.TransactionEvent
	var stop func()
	var err error

	eventsCh, stop, err = s.svc.Client().GetTransactionsStream(ctx)
	if err != nil {
		log.WithError(err).Error("failed to establish initial connection to transaction stream")
		return
	}

	// to avoid creating a deadlock, we collect spent vtxos outpoints while listening to
	// transaction events then, periodically, we cancel pending tasks if any match the collected
	// spent outpoints it avoids locking the delegateMtx for a long time in case a lot of tx are
	// happening in a short period of time
	spentVtxosOutpoints := make([]wire.OutPoint, 0)
	spentVtxosOutpointsMtx := sync.Mutex{}

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// capture the spent vtxos outpoints
				spentVtxosOutpointsMtx.Lock()
				outpoints := spentVtxosOutpoints
				spentVtxosOutpoints = make([]wire.OutPoint, 0)
				spentVtxosOutpointsMtx.Unlock()

				// cancel pending tasks with spent vtxos outpoints
				s.delegateMtx.Lock()

				repo := s.svc.dbSvc.Delegate()
				pendingTaskIds, err := repo.GetPendingTaskIDsByInputs(ctx, outpoints)
				if err != nil {
					log.WithError(err).Warn("failed to get pending tasks by inputs")
				}

				if len(pendingTaskIds) > 0 {
					if err := repo.CancelTasks(ctx, pendingTaskIds...); err != nil {
						log.WithError(err).Warnf("failed to cancel %d tasks", len(pendingTaskIds))
					} else {
						log.Infof(
							"spent vtxos monitor cancelled %d pending tasks", len(pendingTaskIds),
						)
					}
				}

				s.delegateMtx.Unlock()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case event, ok := <-eventsCh:
			if !ok {
				log.Debug("tx event stream closed")
				return
			}
			if event.Err != nil {
				log.WithError(event.Err).Error("error received from transaction stream")
				continue
			}

			spent := getSpentVtxosFromTransactionEvent(event)
			if len(spent) == 0 {
				continue
			}

			spentVtxosOutpointsMtx.Lock()
			spentVtxosOutpoints = append(spentVtxosOutpoints, spent...)
			spentVtxosOutpointsMtx.Unlock()
		}
	}
}

func validateForfeits(
	delegatePubkey *btcec.PublicKey, cfg *clientTypes.Config, forfeitTxs []*psbt.Packet,
) error {
	// TODO validate intent fee

	addr, err := btcutil.DecodeAddress(cfg.ForfeitAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to decode forfeit address: %w", err)
	}

	forfeitOutputScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return fmt.Errorf("failed to create forfeit output script: %w", err)
	}

	for _, forfeit := range forfeitTxs {
		if len(forfeit.UnsignedTx.TxOut) != 2 {
			return fmt.Errorf(
				"invalid number of forfeit outputs: got %d, expected 2",
				len(forfeit.UnsignedTx.TxOut),
			)
		}

		if len(forfeit.UnsignedTx.TxIn) != 1 || len(forfeit.Inputs) != 1 {
			return fmt.Errorf(
				"invalid number of inputs: got %d, expected 1",
				len(forfeit.UnsignedTx.TxIn),
			)
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
			return fmt.Errorf("forfeit tx input witness utxo is nil")
		}

		if len(forfeit.Inputs[0].TaprootScriptSpendSig) != 1 {
			return fmt.Errorf("forfeit tx input has no taproot script spend sig")
		}

		if len(forfeit.Inputs[0].TaprootLeafScript) != 1 {
			return fmt.Errorf("forfeit tx input has no taproot leaf script")
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

		delegateXOnlyKey := schnorr.SerializePubKey(delegatePubkey)
		expectedSigHashType := txscript.SigHashAnyOneCanPay | txscript.SigHashAll

		prevoutMap := make(map[wire.OutPoint]*wire.TxOut)
		forfeitOutpoint := forfeit.UnsignedTx.TxIn[0].PreviousOutPoint
		forfeitSig := forfeit.Inputs[0].TaprootScriptSpendSig[0]
		forfeitLeafScript := forfeit.Inputs[0].TaprootLeafScript[0]

		// verify forfeit sig is related to tapleaf script
		tapLeaf := txscript.NewBaseTapLeaf(forfeitLeafScript.Script)
		tapLeafHash := tapLeaf.TapHash()
		if !bytes.Equal(forfeitSig.LeafHash, tapLeafHash[:]) {
			return fmt.Errorf("forfeit tx input: missing tapleaf script %x", tapLeafHash)
		}

		closure, err := script.DecodeClosure(forfeitLeafScript.Script)
		if err != nil {
			return fmt.Errorf("forfeit tx input: failed to decode closure: %w", err)
		}

		var pubkeys []*btcec.PublicKey

		switch closure := closure.(type) {
		case *script.MultisigClosure:
			pubkeys = closure.PubKeys
		case *script.CLTVMultisigClosure:
			pubkeys = closure.PubKeys
		case *script.ConditionMultisigClosure:
			pubkeys = closure.PubKeys
		default:
			return fmt.Errorf(
				"forfeit tx input: invalid closure type, expected MultisigClosure, "+
					"CLTVMultisigClosure or ConditionMultisigClosure, got %T", closure,
			)
		}

		delegateFound := false
		for _, pubkey := range pubkeys {
			xonlyKey := schnorr.SerializePubKey(pubkey)
			if bytes.Equal(xonlyKey, delegateXOnlyKey) {
				delegateFound = true
				break
			}
		}
		if !delegateFound {
			return fmt.Errorf(
				"forfeit tx input: delegate public key not found in taproot leaf script",
			)
		}

		// verify the signature (must be valid and use sighash type ANYONECANPAY | ALL)
		if forfeitSig.SigHash != expectedSigHashType {
			return fmt.Errorf(
				"forfeit tx input: invalid sighash type, expected AnyoneCanPay | All, got %d",
				forfeitSig.SigHash,
			)
		}

		prevoutMap[forfeitOutpoint] = forfeit.Inputs[0].WitnessUtxo

		// verify all signatures
		prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevoutMap)

		message, err := txscript.CalcTapscriptSignaturehash(
			txscript.NewTxSigHashes(forfeit.UnsignedTx, prevoutFetcher),
			expectedSigHashType, forfeit.UnsignedTx, 0, prevoutFetcher, tapLeaf,
		)
		if err != nil {
			return fmt.Errorf("forfeit tx input: failed to calculate sighash: %w", err)
		}

		signerPublicKey, err := schnorr.ParsePubKey(forfeitSig.XOnlyPubKey)
		if err != nil {
			return fmt.Errorf("forfeit tx input: failed to parse signer public key: %w", err)
		}

		sig, err := schnorr.ParseSignature(forfeitSig.Signature)
		if err != nil {
			return fmt.Errorf("forfeit tx input: failed to parse signature: %w", err)
		}

		if !sig.Verify(message, signerPublicKey) {
			return fmt.Errorf("forfeit tx input: invalid forfeit signature")
		}
	}

	return nil
}
