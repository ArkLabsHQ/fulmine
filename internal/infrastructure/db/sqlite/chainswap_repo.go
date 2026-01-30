package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	log "github.com/sirupsen/logrus"
)

type chainSwapRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewChainSwapRepository(db *sql.DB) (domain.ChainSwapRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open chain swap repository: db is nil")
	}

	return &chainSwapRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *chainSwapRepository) Add(ctx context.Context, swap domain.ChainSwap) error {
	err := r.querier.CreateChainSwap(ctx, queries.CreateChainSwapParams{
		ID:                   swap.Id,
		FromCurrency:         string(swap.From),
		ToCurrency:           string(swap.To),
		Amount:               int64(swap.Amount),
		Status:               int64(swap.Status),
		UserLockupTxID:       toNullableString(swap.UserLockupTxId),
		ServerLockupTxID:     toNullableString(swap.ServerLockupTxId),
		ClaimTxID:            toNullableString(swap.ClaimTxId),
		ClaimPreimage:        swap.ClaimPreimage,
		RefundTxID:           toNullableString(swap.RefundTxId),
		UserBtcLockupAddress: toNullableString(swap.UserBtcLockupAddress),
		ErrorMessage:         toNullableString(swap.ErrorMessage),
	})

	if err != nil {
		return fmt.Errorf("failed to create chain swap: %w", err)
	}

	return nil
}

func (r *chainSwapRepository) Get(ctx context.Context, id string) (*domain.ChainSwap, error) {
	row, err := r.querier.GetChainSwap(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("chain swap %s not found", id)
		}
		return nil, fmt.Errorf("failed to get chain swap: %w", err)
	}

	return toChainSwap(row)
}

func (r *chainSwapRepository) GetAll(ctx context.Context) ([]domain.ChainSwap, error) {
	rows, err := r.querier.ListChainSwaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list chain swaps: %w", err)
	}

	swaps := make([]domain.ChainSwap, 0, len(rows))
	for _, row := range rows {
		swap, err := toChainSwap(row)
		if err != nil {
			log.Warnf("failed to convert chain swap %s: %v", row.ID, err)
			continue
		}
		swaps = append(swaps, *swap)
	}

	return swaps, nil
}

func (r *chainSwapRepository) GetByIDs(ctx context.Context, ids []string) ([]domain.ChainSwap, error) {
	if len(ids) == 0 {
		return []domain.ChainSwap{}, nil
	}

	rows, err := r.querier.ListChainSwapsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list chain swaps by IDs: %w", err)
	}

	swaps := make([]domain.ChainSwap, 0, len(rows))
	for _, row := range rows {
		swap, err := toChainSwap(row)
		if err != nil {
			log.Warnf("failed to convert chain swap %s: %v", row.ID, err)
			continue
		}
		swaps = append(swaps, *swap)
	}

	return swaps, nil
}

func (r *chainSwapRepository) GetByStatus(ctx context.Context, status domain.ChainSwapStatus) ([]domain.ChainSwap, error) {
	rows, err := r.querier.ListChainSwapsByStatus(ctx, int64(status))
	if err != nil {
		return nil, fmt.Errorf("failed to list chain swaps by status: %w", err)
	}

	swaps := make([]domain.ChainSwap, 0, len(rows))
	for _, row := range rows {
		swap, err := toChainSwap(row)
		if err != nil {
			log.Warnf("failed to convert chain swap %s: %v", row.ID, err)
			continue
		}
		swaps = append(swaps, *swap)
	}

	return swaps, nil
}

func (r *chainSwapRepository) Update(ctx context.Context, swap domain.ChainSwap) error {
	err := r.querier.UpdateChainSwap(ctx, queries.UpdateChainSwapParams{
		ID:               swap.Id,
		Status:           int64(swap.Status),
		UserLockupTxID:   toNullableString(swap.UserLockupTxId),
		ServerLockupTxID: toNullableString(swap.ServerLockupTxId),
		ClaimTxID:        toNullableString(swap.ClaimTxId),
		RefundTxID:       toNullableString(swap.RefundTxId),
		ErrorMessage:     toNullableString(swap.ErrorMessage),
	})

	if err != nil {
		return fmt.Errorf("failed to update chain swap: %w", err)
	}

	return nil
}

func (r *chainSwapRepository) Delete(ctx context.Context, id string) error {
	err := r.querier.DeleteChainSwap(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete chain swap: %w", err)
	}

	return nil
}

func (r *chainSwapRepository) Close() {
	if r.db != nil {
		r.db.Close()
	}
}

// Helper functions for conversion

func toChainSwap(row queries.ChainSwap) (*domain.ChainSwap, error) {
	return &domain.ChainSwap{
		Id:                   row.ID,
		From:                 boltz.Currency(row.FromCurrency),
		To:                   boltz.Currency(row.ToCurrency),
		Amount:               uint64(row.Amount),
		Status:               domain.ChainSwapStatus(row.Status),
		UserLockupTxId:       fromNullableString(row.UserLockupTxID),
		ServerLockupTxId:     fromNullableString(row.ServerLockupTxID),
		ClaimTxId:            fromNullableString(row.ClaimTxID),
		ClaimPreimage:        row.ClaimPreimage,
		RefundTxId:           fromNullableString(row.RefundTxID),
		UserBtcLockupAddress: fromNullableString(row.UserBtcLockupAddress),
		ErrorMessage:         fromNullableString(row.ErrorMessage),
		CreatedAt:            fromNullableInt64(row.CreatedAt),
	}, nil
}

func fromNullableInt64(ni sql.NullInt64) int64 {
	if !ni.Valid {
		return 0
	}
	return ni.Int64
}

func toNullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func fromNullableString(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}