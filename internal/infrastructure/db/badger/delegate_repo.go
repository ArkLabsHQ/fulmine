package badgerdb

import (
	"context"
	"encoding/json"
	"errors"
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
) (domain.DelegatorRepository, error) {
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

type outpointJSON struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

type delegateTaskData struct {
	ID                string
	IntentJSON        string
	ForfeitTx         string
	InputJSON         string
	Fee               uint64
	DelegatorPublicKey string
	ScheduledAt       int64
	Status            domain.DelegateTaskStatus
	FailReason        string
}

func (d *delegateTaskData) toDelegateTask() (*domain.DelegateTask, error) {
	var intent domain.Intent
	if err := json.Unmarshal([]byte(d.IntentJSON), &intent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intent: %w", err)
	}

	var opJSON outpointJSON
	if err := json.Unmarshal([]byte(d.InputJSON), &opJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}

	hash, err := chainhash.NewHashFromStr(opJSON.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hash: %w", err)
	}

	return &domain.DelegateTask{
		ID:                d.ID,
		Intent:            intent,
		ForfeitTx:         d.ForfeitTx,
		Input:             wire.OutPoint{
			Hash:  *hash,
			Index: opJSON.Index,
		},
		Fee:               d.Fee,
		DelegatorPublicKey: d.DelegatorPublicKey,
		ScheduledAt:       time.Unix(d.ScheduledAt, 0),
		Status:            d.Status,
		FailReason:        d.FailReason,
	}, nil
}

func toDelegateTaskData(task domain.DelegateTask) delegateTaskData {
	intentJSON, _ := json.Marshal(task.Intent)

	opJSON := outpointJSON{
		Hash:  task.Input.Hash.String(),
		Index: task.Input.Index,
	}
	inputJSON, _ := json.Marshal(opJSON)

	return delegateTaskData{
		ID:                task.ID,
		IntentJSON:        string(intentJSON),
		ForfeitTx:         task.ForfeitTx,
		InputJSON:         string(inputJSON),
		Fee:               task.Fee,
		DelegatorPublicKey: task.DelegatorPublicKey,
		ScheduledAt:       task.ScheduledAt.Unix(),
		Status:            task.Status,
		FailReason:        task.FailReason,
	}
}

func (r *delegateRepository) AddOrUpdate(ctx context.Context, task domain.DelegateTask) error {
	data := toDelegateTaskData(task)

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		// Try to insert first, if it exists, update
		err := r.store.TxInsert(tx, task.ID, data)
		if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
			return err
		}
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return r.store.TxUpdate(tx, task.ID, data)
		}
		return nil
	}

	// Try to insert first, if it exists, update
	err := r.store.Insert(task.ID, data)
	if err != nil && !errors.Is(err, badgerhold.ErrKeyExists) {
		return err
	}
	if errors.Is(err, badgerhold.ErrKeyExists) {
		return r.store.Update(task.ID, data)
	}

	return nil
}

func (r *delegateRepository) GetByID(ctx context.Context, id string) (*domain.DelegateTask, error) {
	var data delegateTaskData
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
	var allTasks []delegateTaskData
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

func (r *delegateRepository) Close() {
	// nolint:all
	r.store.Close()
}
