package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"

	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
)

type subscribedScriptRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewSubscribedScriptRepository(db *sql.DB) (domain.SubscribedScriptRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open subscribed script repository: db is nil")
	}

	return &subscribedScriptRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *subscribedScriptRepository) Add(ctx context.Context, scripts []string) (count int, err error) {
	count = 0
	txBody := func(querierWithTx *queries.Queries) error {
		for _, script := range scripts {
			err := querierWithTx.InsertSubscribedScript(ctx, script)
			if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
				// If the script already exists, we can skip it
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to insert script %s: %w", script, err)
			}
			count++
		}
		return nil
	}

	err = execTx(ctx, r.db, txBody)
	if err != nil {
		return 0, fmt.Errorf("failed to execute transaction: %w", err)
	}

	return count, nil
}

func (r *subscribedScriptRepository) Get(ctx context.Context) ([]string, error) {
	rows, err := r.querier.ListSubscribedScript(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return []string{}, nil
		}
		return nil, err
	}

	return rows, nil
}

func (r *subscribedScriptRepository) Delete(ctx context.Context, scripts []string) (count int, err error) {
	count = 0
	txBody := func(querierWithTx *queries.Queries) error {
		for _, script := range scripts {
			// Check if the script exists before trying to delete it
			_, err := querierWithTx.GetSubscribedScript(ctx, script)
			if errors.Is(err, sql.ErrNoRows) {
				// If the script does not exist, we can skip it
				continue
			}

			err = querierWithTx.DeleteSubscribedScript(ctx, script)

			if err != nil {
				return fmt.Errorf("failed to delete script %s: %w", script, err)
			}
			count++
		}
		return nil
	}

	err = execTx(ctx, r.db, txBody)
	if err != nil {
		return 0, fmt.Errorf("failed to execute transaction: %w", err)
	}

	return count, nil
}

func (r *subscribedScriptRepository) Close() {
	// nolint
	r.db.Close()
}
