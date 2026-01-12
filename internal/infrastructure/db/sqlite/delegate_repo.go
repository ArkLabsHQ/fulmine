package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type delegateRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewDelegateRepository(db *sql.DB) (domain.DelegatorRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open delegate repository: db is nil")
	}

	return &delegateRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

type outpointJSON struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

func (r *delegateRepository) AddOrUpdate(ctx context.Context, task domain.DelegateTask) error {
	intentJSON, err := json.Marshal(task.Intent)
	if err != nil {
		return fmt.Errorf("failed to marshal intent: %w", err)
	}

	opJSON := outpointJSON{
		Hash:  task.Input.Hash.String(),
		Index: task.Input.Index,
	}
	inputJSON, err := json.Marshal(opJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	return r.querier.UpsertDelegateTask(ctx, queries.UpsertDelegateTaskParams{
		ID:                task.ID,
		IntentJson:        string(intentJSON),
		ForfeitTx:         task.ForfeitTx,
		InputsJson:        string(inputJSON),
		Fee:               int64(task.Fee),
		DelegatorPublicKey: task.DelegatorPublicKey,
		ScheduledAt:       task.ScheduledAt.Unix(),
		Status:            string(task.Status),
		FailReason:        task.FailReason,
	})
}

func (r *delegateRepository) GetByID(ctx context.Context, id string) (*domain.DelegateTask, error) {
	row, err := r.querier.GetDelegateTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("delegate task not found for id: %s", id)
		}
		return nil, err
	}

	var intent domain.Intent
	if err := json.Unmarshal([]byte(row.IntentJson), &intent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intent: %w", err)
	}

	var opJSON outpointJSON
	if err := json.Unmarshal([]byte(row.InputsJson), &opJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}

	hash, err := chainhash.NewHashFromStr(opJSON.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hash: %w", err)
	}

	return &domain.DelegateTask{
		ID:                row.ID,
		Intent:            intent,
		ForfeitTx:         row.ForfeitTx,
		Input:             wire.OutPoint{
			Hash:  *hash,
			Index: opJSON.Index,
		},
		Fee:               uint64(row.Fee),
		DelegatorPublicKey: row.DelegatorPublicKey,
		ScheduledAt:       time.Unix(row.ScheduledAt, 0),
		Status:            domain.DelegateTaskStatus(row.Status),
		FailReason:        row.FailReason,
	}, nil
}

func (r *delegateRepository) GetAllPending(ctx context.Context) ([]domain.PendingDelegateTask, error) {
	rows, err := r.querier.ListDelegateTaskPending(ctx)
	if err != nil {
		return nil, err
	}

	tasks := make([]domain.PendingDelegateTask, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, domain.PendingDelegateTask{
			ID:          row.ID,
			ScheduledAt: time.Unix(row.ScheduledAt, 0),
		})
	}

	return tasks, nil
}

func (r *delegateRepository) GetPendingTaskByInput(ctx context.Context, input wire.OutPoint) ([]domain.DelegateTask, error) {
	rows, err := r.querier.GetPendingTaskByInput(ctx, queries.GetPendingTaskByInputParams{
		InputsJson:   input.Hash.String(),
		InputsJson_2: fmt.Sprintf("%d", input.Index),
	})
	if err != nil {
		return nil, err
	}

	tasks := make([]domain.DelegateTask, 0, len(rows))
	for _, row := range rows {
		var intent domain.Intent
		if err := json.Unmarshal([]byte(row.IntentJson), &intent); err != nil {
			return nil, fmt.Errorf("failed to unmarshal intent: %w", err)
		}

		var opJSON outpointJSON
		if err := json.Unmarshal([]byte(row.InputsJson), &opJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}

		hash, err := chainhash.NewHashFromStr(opJSON.Hash)
		if err != nil {
			return nil, fmt.Errorf("failed to parse hash: %w", err)
		}

		tasks = append(tasks, domain.DelegateTask{
			ID:                row.ID,
			Intent:            intent,
			ForfeitTx:         row.ForfeitTx,
			Input:             wire.OutPoint{
				Hash:  *hash,
				Index: opJSON.Index,
			},
			Fee:               uint64(row.Fee),
			DelegatorPublicKey: row.DelegatorPublicKey,
			ScheduledAt:       time.Unix(row.ScheduledAt, 0),
			Status:            domain.DelegateTaskStatus(row.Status),
			FailReason:        row.FailReason,
		})
	}

	return tasks, nil
}

func (r *delegateRepository) Close() {
	_ = r.db.Close()
}
