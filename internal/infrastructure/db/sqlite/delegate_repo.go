package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/json"
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

func NewDelegateRepository(db *sql.DB) (domain.DelegateRepository, error) {
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
				InputHash:  input.Hash.String(),
				InputIndex: int64(input.Index),
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
		if row.InputHash.Valid && row.InputIndex.Valid {
			hash, err := chainhash.NewHashFromStr(row.InputHash.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse hash: %w", err)
			}
			outpoint := wire.OutPoint{
				Hash:  *hash,
				Index: uint32(row.InputIndex.Int64),
			}
			inputs = append(inputs, outpoint)
			
			if row.ForfeitTx.Valid && row.ForfeitTx.String != "" {
				forfeitTxs[outpoint] = row.ForfeitTx.String
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
	searchInputsJSON := make([]outpointJSON, len(inputs))
	for i, input := range inputs {
		searchInputsJSON[i] = outpointJSON{
			Hash:  input.Hash.String(),
			Index: input.Index,
		}
	}
	searchInputsJSONBytes, err := json.Marshal(searchInputsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search inputs: %w", err)
	}

	// Can't use sqlc-generated query because it doesn't generate the search_input parameter
	query := `
		SELECT DISTINCT dt.id
		FROM delegate_task dt
		INNER JOIN delegate_task_input dti ON dt.id = dti.task_id
		WHERE dt.status = 0
		  AND EXISTS (
		    SELECT 1 FROM json_each(?) AS search_input
		    WHERE dti.input_hash = json_extract(search_input.value, '$.hash')
		      AND dti.input_index = CAST(json_extract(search_input.value, '$.index') AS INTEGER)
		  )
	`
	rows, err := r.db.QueryContext(ctx, query, string(searchInputsJSONBytes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	taskIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan task ID: %w", err)
		}
		taskIDs = append(taskIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return taskIDs, nil
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
