package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
)

type bancoPairRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewBancoPairRepository(db *sql.DB) (domain.BancoPairRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open banco pair repository: db is nil")
	}
	return &bancoPairRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *bancoPairRepository) Add(ctx context.Context, pair domain.BancoPair) error {
	var invertPrice int64
	if pair.InvertPrice {
		invertPrice = 1
	}
	err := r.querier.InsertBancoPair(ctx, queries.InsertBancoPairParams{
		Pair:         pair.Pair,
		QuoteAssetID: pair.QuoteAssetID,
		MinAmount:    int64(pair.MinAmount),
		MaxAmount:    int64(pair.MaxAmount),
		PriceFeed:    pair.PriceFeed,
		InvertPrice:  invertPrice,
	})
	if err != nil {
		return fmt.Errorf("failed to insert banco pair: %w", err)
	}
	return nil
}

func (r *bancoPairRepository) Update(ctx context.Context, pair domain.BancoPair) error {
	var invertPrice int64
	if pair.InvertPrice {
		invertPrice = 1
	}
	err := r.querier.UpdateBancoPair(ctx, queries.UpdateBancoPairParams{
		QuoteAssetID: pair.QuoteAssetID,
		MinAmount:    int64(pair.MinAmount),
		MaxAmount:    int64(pair.MaxAmount),
		PriceFeed:    pair.PriceFeed,
		InvertPrice:  invertPrice,
		Pair:         pair.Pair,
	})
	if err != nil {
		return fmt.Errorf("failed to update banco pair: %w", err)
	}
	return nil
}

func (r *bancoPairRepository) Remove(ctx context.Context, pairName string) error {
	err := r.querier.DeleteBancoPair(ctx, pairName)
	if err != nil {
		return fmt.Errorf("failed to delete banco pair: %w", err)
	}
	return nil
}

func (r *bancoPairRepository) List(ctx context.Context) ([]domain.BancoPair, error) {
	rows, err := r.querier.ListBancoPairs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list banco pairs: %w", err)
	}

	pairs := make([]domain.BancoPair, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, domain.BancoPair{
			Pair:         row.Pair,
			QuoteAssetID: row.QuoteAssetID,
			MinAmount:    uint64(row.MinAmount),
			MaxAmount:    uint64(row.MaxAmount),
			PriceFeed:    row.PriceFeed,
			InvertPrice:  row.InvertPrice != 0,
		})
	}
	return pairs, nil
}

func (r *bancoPairRepository) Close() {
	// db is shared across repos, closed by the main service
}
