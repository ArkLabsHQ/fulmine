package sqlitedb

import (
	"context"
	"database/sql"
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

func (r *subscribedScriptRepository) Add(ctx context.Context, scripts []string) error {
	row, err := r.querier.GetSubscribedScript(ctx)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get subscribed scripts: %w", err)
	}

	var decodedScripts []string
	if row.Scripts != "" {
		decodedScripts = strings.Split(row.Scripts, ",")
	} else {
		decodedScripts = []string{}
	}
	aggregatedScripts := append(decodedScripts, scripts...)
	encodedScript := strings.Join(aggregatedScripts, ",")
	return r.querier.InsertSubscribedScript(ctx, encodedScript)
}

func (r *subscribedScriptRepository) Get(ctx context.Context) ([]string, error) {
	row, err := r.querier.GetSubscribedScript(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return []string{}, nil
		}
		return nil, err
	}

	decodedScripts := strings.Split(row.Scripts, ",")
	return decodedScripts, nil
}

func (r *subscribedScriptRepository) Delete(ctx context.Context, scripts []string) (count int, err error) {
	row, err := r.querier.GetSubscribedScript(ctx)
	if err != nil {
		return 0, err
	}
	decodedScripts := strings.Split(row.Scripts, ",")

	scriptSet := make(map[string]struct{})
	for _, script := range scripts {
		scriptSet[script] = struct{}{}
	}

	updatedScripts := []string{}
	for _, script := range decodedScripts {
		if _, exists := scriptSet[script]; !exists {
			updatedScripts = append(updatedScripts, script)
		} else {
			count++
		}
	}

	encodedScripts := strings.Join(updatedScripts, ",")
	err = r.querier.InsertSubscribedScript(ctx, encodedScripts)
	if err != nil {
		return 0, fmt.Errorf("failed to update subscribed scripts: %w", err)
	}

	return count, nil
}

func (r *subscribedScriptRepository) Close() {
	// nolint
	r.db.Close()
}
