package badgerdb

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

const (
	delegateDir = "delegate"
)

type delegateRepository struct {
	store *badgerhold.Store
}

func NewDelegateRepository(
	baseDir string, logger badger.Logger,
) (domain.DelegateRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, delegateDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open delegate store: %s", err)
	}
	return &delegateRepository{store}, nil
}


func (r *delegateRepository) Add(ctx context.Context, task domain.DelegateTask) error {
	data := toDelegateTaskData(task)

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		return r.store.TxInsert(tx, task.ID, data)
	}

	return r.store.Insert(task.ID, data)
}

func (r *delegateRepository) GetByID(ctx context.Context, id string) (*domain.DelegateTask, error) {
	var data delegateTaskDTO
	var err error

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = r.store.TxGet(tx, id, &data)
	} else {
		err = r.store.Get(id, &data)
	}

	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("delegate task not found for id: %s", id)
		}
		return nil, err
	}

	return data.toDelegateTask()
}

func (r *delegateRepository) GetAllPending(ctx context.Context) ([]domain.PendingDelegateTask, error) {
	var allTasks []delegateTaskDTO
	var err error

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = r.store.TxFind(tx, &allTasks, badgerhold.Where("Status").Eq(domain.DelegateTaskStatusPending))
	} else {
		err = r.store.Find(&allTasks, badgerhold.Where("Status").Eq(domain.DelegateTaskStatusPending))
	}

	if err != nil {
		return nil, err
	}

	tasks := make([]domain.PendingDelegateTask, len(allTasks))
	for i, data := range allTasks {
		tasks[i] = domain.PendingDelegateTask{
			ID:          data.ID,
			ScheduledAt: time.Unix(data.ScheduledAt, 0),
		}
	}

	return tasks, nil
}

func (r *delegateRepository) GetPendingTaskIDsByInputs(ctx context.Context, inputs []wire.OutPoint) ([]string, error) {
	var allTasks []delegateTaskDTO
	var err error

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = r.store.TxFind(tx, &allTasks, badgerhold.Where("Status").Eq(domain.DelegateTaskStatusPending))
	} else {
		err = r.store.Find(&allTasks, badgerhold.Where("Status").Eq(domain.DelegateTaskStatusPending))
	}

	if err != nil {
		return nil, err
	}

	inputSet := make(map[string]bool)
	for _, input := range inputs {
		key := fmt.Sprintf("%s:%d", input.Hash.String(), input.Index)
		inputSet[key] = true
	}

	taskIDs := make([]string, 0)
	for _, data := range allTasks {
		var opJSONs []outpointJSON
		if err := json.Unmarshal([]byte(data.InputJSON), &opJSONs); err != nil {
			continue 
		}

		hasOverlap := false
		for _, opJSON := range opJSONs {
			key := fmt.Sprintf("%s:%d", opJSON.Hash, opJSON.Index)
			if inputSet[key] {
				hasOverlap = true
				break
			}
		}

		if hasOverlap {
			taskIDs = append(taskIDs, data.ID)
		}
	}

	return taskIDs, nil
}

func (r *delegateRepository) CancelTasks(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil 
	}

	for _, id := range ids {
		task, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}

		if task.Status != domain.DelegateTaskStatusPending {
			continue
		}

		task.Status = domain.DelegateTaskStatusCancelled
		data := toDelegateTaskData(*task)

		var updateErr error
		if ctx.Value("tx") != nil {
			tx := ctx.Value("tx").(*badger.Txn)
			updateErr = r.store.TxUpdate(tx, id, data)
		} else {
			updateErr = r.store.Update(id, data)
		}

		if updateErr != nil {
			return fmt.Errorf("failed to cancel task %s: %w", id, updateErr)
		}
	}

	return nil
}

func (r *delegateRepository) CompleteTasks(ctx context.Context, commitmentTxid string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		task, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}

		if task.Status != domain.DelegateTaskStatusPending {
			continue
		}

		task.Status = domain.DelegateTaskStatusCompleted
		task.CommitmentTxid = commitmentTxid
		data := toDelegateTaskData(*task)

		var updateErr error
		if ctx.Value("tx") != nil {
			tx := ctx.Value("tx").(*badger.Txn)
			updateErr = r.store.TxUpdate(tx, id, data)
		} else {
			updateErr = r.store.Update(id, data)
		}

		if updateErr != nil {
			return fmt.Errorf("failed to mark task %s as done: %w", id, updateErr)
		}	
	}

	return nil
}

func (r *delegateRepository) FailTasks(ctx context.Context, reason string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		task, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}

		if task.Status != domain.DelegateTaskStatusPending {
			continue
		}

		task.Status = domain.DelegateTaskStatusFailed
		task.FailReason = reason
		data := toDelegateTaskData(*task)

		var updateErr error
		if ctx.Value("tx") != nil {
			tx := ctx.Value("tx").(*badger.Txn)
			updateErr = r.store.TxUpdate(tx, id, data)
		} else {
			updateErr = r.store.Update(id, data)
		}

		if updateErr != nil {
			return fmt.Errorf("failed to mark task %s as failed: %w", id, updateErr)
		}
	}

	return nil
}

func (r *delegateRepository) Close() {
	// nolint:all
	r.store.Close()
}

type outpointJSON struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

type delegateTaskDTO struct {
	ID                string
	IntentJSON        string
	InputJSON         string
	ForfeitTxsJSON    string // JSON map of outpoint -> forfeit_tx
	Fee               uint64
	DelegatorPublicKey string
	ScheduledAt       int64
	Status            domain.DelegateTaskStatus
	FailReason        string
	CommitmentTxid    string
}

func (d *delegateTaskDTO) toDelegateTask() (*domain.DelegateTask, error) {
	var intent domain.Intent
	if err := json.Unmarshal([]byte(d.IntentJSON), &intent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intent: %w", err)
	}

	var opJSONs []outpointJSON
	if err := json.Unmarshal([]byte(d.InputJSON), &opJSONs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
	}

	inputs := make([]wire.OutPoint, len(opJSONs))
	for i, opJSON := range opJSONs {
		hash, err := chainhash.NewHashFromStr(opJSON.Hash)
		if err != nil {
			return nil, fmt.Errorf("failed to parse hash for input %d: %w", i, err)
		}
		inputs[i] = wire.OutPoint{
			Hash:  *hash,
			Index: opJSON.Index,
		}
	}
	intent.Inputs = inputs

	var forfeitTxs map[wire.OutPoint]string
	if len(d.ForfeitTxsJSON) > 0 {
		var forfeitTxsJSON map[string]string
		if err := json.Unmarshal([]byte(d.ForfeitTxsJSON), &forfeitTxsJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal forfeit txs: %w", err)
		}
		forfeitTxs = make(map[wire.OutPoint]string)
		for key, forfeitTx := range forfeitTxsJSON {
			// key format: "hash:index"
			lastColonIdx := -1
			for i := len(key) - 1; i >= 0; i-- {
				if key[i] == ':' {
					lastColonIdx = i
					break
				}
			}
			if lastColonIdx == -1 {
				return nil, fmt.Errorf("failed to parse forfeit tx key %s: no colon found", key)
			}
			hashStr := key[:lastColonIdx]
			var index uint32
			if _, err := fmt.Sscanf(key[lastColonIdx+1:], "%d", &index); err != nil {
				return nil, fmt.Errorf("failed to parse forfeit tx key %s: invalid index: %w", key, err)
			}
			hash, err := chainhash.NewHashFromStr(hashStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse hash in forfeit tx key %s: %w", key, err)
			}
			forfeitTxs[wire.OutPoint{Hash: *hash, Index: index}] = forfeitTx
		}
	} else {
		forfeitTxs = make(map[wire.OutPoint]string)
	}

	return &domain.DelegateTask{
		ID:                d.ID,
		Intent:            intent,
		ForfeitTxs:        forfeitTxs,
		Fee:               d.Fee,
		DelegatorPublicKey: d.DelegatorPublicKey,
		ScheduledAt:       time.Unix(d.ScheduledAt, 0),
		Status:            d.Status,
		FailReason:        d.FailReason,
		CommitmentTxid:    d.CommitmentTxid,
	}, nil
}

func toDelegateTaskData(task domain.DelegateTask) delegateTaskDTO {
	intentJSON, _ := json.Marshal(task.Intent)

	opJSONs := make([]outpointJSON, len(task.Intent.Inputs))
	for i, input := range task.Intent.Inputs {
		opJSONs[i] = outpointJSON{
			Hash:  input.Hash.String(),
			Index: input.Index,
		}
	}
	inputJSON, _ := json.Marshal(opJSONs)

	forfeitTxsJSON := make(map[string]string)
	for outpoint, forfeitTx := range task.ForfeitTxs {
		key := fmt.Sprintf("%s:%d", outpoint.Hash.String(), outpoint.Index)
		forfeitTxsJSON[key] = forfeitTx
	}
	forfeitTxsJSONBytes, _ := json.Marshal(forfeitTxsJSON)

	return delegateTaskDTO{
		ID:                task.ID,
		IntentJSON:        string(intentJSON),
		InputJSON:         string(inputJSON),
		ForfeitTxsJSON:    string(forfeitTxsJSONBytes),
		Fee:               task.Fee,
		DelegatorPublicKey: task.DelegatorPublicKey,
		ScheduledAt:       task.ScheduledAt.Unix(),
		Status:            task.Status,
		FailReason:        task.FailReason,
		CommitmentTxid:    task.CommitmentTxid,
	}
}
