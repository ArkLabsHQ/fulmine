package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
)

type swapRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewSwapRepository(db *sql.DB) (domain.SwapRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open vtxo rollover repository: db is nil")
	}

	return &swapRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *swapRepository) Add(ctx context.Context, swap domain.Swap) error {

	return r.querier.CreateSwap(ctx, queries.CreateSwapParams{
		ID:      swap.Id,
		Amount:  int64(swap.Amount),
		Date:    swap.Date.Format(time.DateTime),
		To:      string(swap.To),
		From:    string(swap.From),
		Status:  int64(swap.Status),
		Invoice: swap.Invoice,
		VhltcID: swap.VHltcId,
	})
}

func (r *swapRepository) Get(ctx context.Context, swapId string) (*domain.Swap, error) {
	row, err := r.querier.GetSwap(ctx, swapId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("swap for the Id: %s, not found", swapId)
		}
		return nil, err
	}

	return toSwap(row)
}

func (r *swapRepository) GetAll(ctx context.Context) ([]domain.Swap, error) {
	rows, err := r.querier.ListSwaps(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.Swap
	for _, row := range rows {
		swap, err := toSwap(row)
		if err != nil {
			return nil, err
		}
		results = append(results, *swap)
	}
	return results, nil
}

func (r *swapRepository) Delete(ctx context.Context, swapId string) error {
	return r.querier.DeleteSwap(ctx, swapId)
}

func (r *swapRepository) Close() {
	_ = r.db.Close()
}

func toSwap(row queries.Swap) (*domain.Swap, error) {
	date, err := time.Parse(time.DateTime, row.Date)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	return &domain.Swap{
		Id:      row.ID,
		Amount:  uint64(row.Amount),
		Date:    date,
		To:      boltz.Currency(row.To),
		From:    boltz.Currency(row.From),
		Status:  domain.SwapStatus(row.Status),
		Invoice: row.Invoice,
		VHltcId: row.VhltcID,
	}, nil
}
