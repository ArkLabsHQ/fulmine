package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/btcsuite/btcd/wire"
)

type delegateRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewDelegateRepository(db *sql.DB) (domain.DelegateRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open delegate repository: db is nil")
	}

	return &delegateRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *delegateRepository) Add(ctx context.Context, task domain.DelegateTask) error {
	txBody := func(querierWithTx *queries.Queries) error {
		if err := querierWithTx.InsertDelegateTask(ctx, queries.InsertDelegateTaskParams{
			ID:                task.ID,
			IntentTxid:        task.Intent.Txid,
			IntentMessage:     task.Intent.Message,
			IntentProof:       task.Intent.Proof,
			Fee:               int64(task.Fee),
			DelegatorPublicKey: task.DelegatorPublicKey,
			ScheduledAt:       task.ScheduledAt.Unix(),
			Status:            int64(task.Status),
		}); err != nil {
			return fmt.Errorf("failed to insert delegate task: %w", err)
		}

		for _, input := range task.Intent.Inputs {
			forfeitTx, ok := task.ForfeitTxs[input]
			
			if err := querierWithTx.InsertDelegateTaskInput(ctx, queries.InsertDelegateTaskInputParams{
				TaskID:     task.ID,
				Outpoint:   input.String(),
				ForfeitTx:  sql.NullString{String: forfeitTx, Valid: ok},
			}); err != nil {
				return fmt.Errorf("failed to insert input: %w", err)
			}
		}

		return nil
	}

	return execTx(ctx, r.db, txBody)
}

func (r *delegateRepository) GetByID(ctx context.Context, id string) (*domain.DelegateTask, error) {
	rows, err := r.querier.GetDelegateTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("delegate task not found for id: %s", id)
	}

	firstRow := rows[0]

	inputs := make([]wire.OutPoint, 0)
	forfeitTxs := make(map[wire.OutPoint]string)
	for _, row := range rows {
		if row.Outpoint.Valid {
			outpoint, err := wire.NewOutPointFromString(row.Outpoint.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse outpoint: %w", err)
			}
			inputs = append(inputs, *outpoint)
			
			if row.ForfeitTx.Valid && row.ForfeitTx.String != "" {
				forfeitTxs[*outpoint] = row.ForfeitTx.String
			}
		}
	}

	intent := domain.Intent{
		Message: firstRow.IntentMessage,
		Proof:   firstRow.IntentProof,
		Txid:    firstRow.IntentTxid,
		Inputs:  inputs,
	}

	return &domain.DelegateTask{
		ID:                firstRow.ID,
		Intent:            intent,
		ForfeitTxs:        forfeitTxs,
		Fee:               uint64(firstRow.Fee),
		DelegatorPublicKey: firstRow.DelegatorPublicKey,
		ScheduledAt:       time.Unix(firstRow.ScheduledAt, 0),
		Status:            domain.DelegateTaskStatus(firstRow.Status),
		FailReason:        firstRow.FailReason.String,
		CommitmentTxid:    firstRow.CommitmentTxid.String,
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

func (r *delegateRepository) GetPendingTaskByIntentTxID(ctx context.Context, txid string) (*domain.PendingDelegateTask, error) {
	row, err := r.querier.GetPendingTaskByIntentTxID(ctx, txid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("pending delegate task not found for intent txid: %s", txid)
		}
		return nil, err
	}

	return &domain.PendingDelegateTask{
		ID:          row.ID,
		ScheduledAt: time.Unix(row.ScheduledAt, 0),
	}, nil
}

func (r *delegateRepository) GetPendingTaskIDsByInputs(ctx context.Context, inputs []wire.OutPoint) ([]string, error) {
	if len(inputs) == 0 {
		return []string{}, nil
	}

	outpoints := make([]string, len(inputs))
	for i, input := range inputs {
		outpoints[i] = input.String()
	}

	return r.querier.GetPendingTaskIDsByInputs(ctx, outpoints)
}

func (r *delegateRepository) GetAll(ctx context.Context, status domain.DelegateTaskStatus, limit int, offset int) ([]domain.DelegateTask, error) {
	rows, err := r.querier.ListDelegateTasks(ctx, queries.ListDelegateTasksParams{
		Status: int64(status),
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	taskMap := make(map[string]*domain.DelegateTask)
	taskOrder := make([]string, 0)
	for _, row := range rows {
		task, exists := taskMap[row.ID]
		if !exists {
			inputs := make([]wire.OutPoint, 0)
			forfeitTxs := make(map[wire.OutPoint]string)
			
			if row.Outpoint.Valid {
				outpoint, err := wire.NewOutPointFromString(row.Outpoint.String)
				if err != nil {
					return nil, fmt.Errorf("failed to parse outpoint: %w", err)
				}
				inputs = append(inputs, *outpoint)
				
				if row.ForfeitTx.Valid && row.ForfeitTx.String != "" {
					forfeitTxs[*outpoint] = row.ForfeitTx.String
				}
			}

			intent := domain.Intent{
				Message: row.IntentMessage,
				Proof:   row.IntentProof,
				Txid:    row.IntentTxid,
				Inputs:  inputs,
			}

			task = &domain.DelegateTask{
				ID:                row.ID,
				Intent:            intent,
				ForfeitTxs:        forfeitTxs,
				Fee:               uint64(row.Fee),
				DelegatorPublicKey: row.DelegatorPublicKey,
				ScheduledAt:       time.Unix(row.ScheduledAt, 0),
				Status:            domain.DelegateTaskStatus(row.Status),
				FailReason:        row.FailReason.String,
				CommitmentTxid:    row.CommitmentTxid.String,
			}
			taskMap[row.ID] = task
			taskOrder = append(taskOrder, row.ID)
		} else {
			if row.Outpoint.Valid {
				outpoint, err := wire.NewOutPointFromString(row.Outpoint.String)
				if err != nil {
					return nil, fmt.Errorf("failed to parse outpoint: %w", err)
				}
				task.Intent.Inputs = append(task.Intent.Inputs, *outpoint)
				
				if row.ForfeitTx.Valid && row.ForfeitTx.String != "" {
					task.ForfeitTxs[*outpoint] = row.ForfeitTx.String
				}
			}
		}
	}

	tasks := make([]domain.DelegateTask, 0, len(taskMap))
	for _, taskID := range taskOrder {
		if task, exists := taskMap[taskID]; exists {
			tasks = append(tasks, *task)
		}
	}

	return tasks, nil
}

func (r *delegateRepository) CancelTasks(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil // Nothing to cancel
	}

	err := r.querier.CancelDelegateTasks(ctx, ids)
	if err != nil {
		return fmt.Errorf("failed to cancel delegate tasks: %w", err)
	}

	return nil
}

func (r *delegateRepository) CompleteTasks(ctx context.Context, commitmentTxid string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	err := r.querier.SuccessDelegateTasks(ctx, queries.SuccessDelegateTasksParams{
		CommitmentTxid: sql.NullString{String: commitmentTxid, Valid: commitmentTxid != ""},
		Ids:            ids,
	})
	if err != nil {
		return fmt.Errorf("failed to mark delegate tasks as done: %w", err)
	}

	return nil
}

func (r *delegateRepository) FailTasks(ctx context.Context, reason string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	err := r.querier.FailDelegateTasks(ctx, queries.FailDelegateTasksParams{
		FailReason: sql.NullString{String: reason, Valid: len(reason) > 0},
		Ids: ids,
	})
	if err != nil {
		return fmt.Errorf("failed to mark delegate tasks as failed: %w", err)
	}

	return nil
}

func (r *delegateRepository) Close() {
	_ = r.db.Close()
}
