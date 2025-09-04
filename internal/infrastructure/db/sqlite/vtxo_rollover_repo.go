package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
)

type vtxoRolloverRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewVtxoRolloverRepository(db *sql.DB) (domain.VtxoRolloverRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open vtxo rollover repository: db is nil")
	}

	return &vtxoRolloverRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *vtxoRolloverRepository) AddTarget(ctx context.Context, target domain.VtxoRolloverTarget) error {
	timeoutContext, cancel := context.WithTimeout(ctx, ports.DefaultDbTimeout)
	defer cancel()

	treeJSON, err := json.Marshal(target.TaprootTree)
	if err != nil {
		return err
	}
	return r.querier.UpsertVtxoRollover(timeoutContext, queries.UpsertVtxoRolloverParams{
		Address:            target.Address,
		TaprootTree:        string(treeJSON),
		DestinationAddress: target.DestinationAddress,
	})
}

func (r *vtxoRolloverRepository) GetTarget(ctx context.Context, address string) (*domain.VtxoRolloverTarget, error) {
	timeoutContext, cancel := context.WithTimeout(ctx, ports.DefaultDbTimeout)
	defer cancel()

	row, err := r.querier.GetVtxoRollover(timeoutContext, address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("rollover target not found for address: %s", address)
		}
		return nil, err
	}
	var tree []string
	if err := json.Unmarshal([]byte(row.TaprootTree), &tree); err != nil {
		return nil, err
	}
	return &domain.VtxoRolloverTarget{
		Address:            row.Address,
		TaprootTree:        tree,
		DestinationAddress: row.DestinationAddress,
	}, nil
}

func (r *vtxoRolloverRepository) GetAllTargets(ctx context.Context) ([]domain.VtxoRolloverTarget, error) {
	timeoutContext, cancel := context.WithTimeout(ctx, ports.DefaultDbTimeout)
	defer cancel()

	rows, err := r.querier.ListVtxoRollover(timeoutContext)
	if err != nil {
		return nil, err
	}
	var results []domain.VtxoRolloverTarget
	for _, row := range rows {
		var tree []string
		if err := json.Unmarshal([]byte(row.TaprootTree), &tree); err != nil {
			return nil, err
		}
		results = append(results, domain.VtxoRolloverTarget{
			Address:            row.Address,
			TaprootTree:        tree,
			DestinationAddress: row.DestinationAddress,
		})
	}
	return results, nil
}

func (r *vtxoRolloverRepository) DeleteTarget(ctx context.Context, address string) error {
	timeoutContext, cancel := context.WithTimeout(ctx, ports.DefaultDbTimeout)
	defer cancel()

	return r.querier.DeleteVtxoRollover(timeoutContext, address)
}

func (r *vtxoRolloverRepository) Close() {
	_ = r.db.Close()
}
