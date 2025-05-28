package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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
	txBody := func(querierWithTx *queries.Queries) error {
		optsParams := toOptParams(swap.VhtlcOpts)
		preimageHash := optsParams.PreimageHash

		if err := querierWithTx.InsertVHTLC(ctx, optsParams); err != nil {
			if sqlErr, ok := err.(*sqlite.Error); ok {
				if sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
					return fmt.Errorf("vHTLC with preimage hash %s already exists", optsParams.PreimageHash)
				}
			}
			return fmt.Errorf("failed to insert vhtlc: %s", err)
		}

		if err := querierWithTx.CreateSwap(ctx, queries.CreateSwapParams{
			ID:           swap.Id,
			Amount:       int64(swap.Amount),
			Timestamp:    swap.Timestamp,
			ToCurrency:   string(swap.To),
			FromCurrency: string(swap.From),
			Status:       int64(swap.Status),
			Invoice:      swap.Invoice,
			FundingTxID:  swap.FundingTxId,
			RedeemTxID:   swap.RedeemTxId,
			VhtlcID:      preimageHash,
		}); err != nil {
			return fmt.Errorf("failed to insert swap: %s", err)
		}
		return nil
	}

	return execTx(ctx, r.db, txBody)
}

func (r *swapRepository) Get(ctx context.Context, swapId string) (*domain.Swap, error) {
	row, err := r.querier.GetSwap(ctx, swapId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("swap %s not found", swapId)
		}
		return nil, err
	}

	return toSwap(row.Swap, row.Vhtlc)
}

func (r *swapRepository) GetAll(ctx context.Context) ([]domain.Swap, error) {
	rows, err := r.querier.ListSwaps(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.Swap
	for _, row := range rows {
		swap, err := toSwap(row.Swap, row.Vhtlc)
		if err != nil {
			return nil, err
		}
		results = append(results, *swap)
	}
	return results, nil
}

func (r *swapRepository) Close() {
	// nolint
	r.db.Close()
}

func toSwap(swap queries.Swap, vhtlc queries.Vhtlc) (*domain.Swap, error) {
	vhtlcOpts, err := toOpts(vhtlc)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vhtlc opts: %w", err)
	}

	return &domain.Swap{
		Id:          swap.ID,
		Amount:      uint64(swap.Amount),
		Timestamp:   swap.Timestamp,
		To:          boltz.Currency(swap.ToCurrency),
		From:        boltz.Currency(swap.FromCurrency),
		Status:      domain.SwapStatus(swap.Status),
		Invoice:     swap.Invoice,
		FundingTxId: swap.FundingTxID,
		RedeemTxId:  swap.RedeemTxID,
		VhtlcOpts:   *vhtlcOpts,
	}, nil
}
