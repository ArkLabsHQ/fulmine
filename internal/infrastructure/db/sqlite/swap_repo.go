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
		vhtlcRow := toVhtlcRow(swap.Vhtlc)

		if err := querierWithTx.InsertVHTLC(ctx, vhtlcRow); err != nil {
			if sqlErr, ok := err.(*sqlite.Error); ok {
				if sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
					return fmt.Errorf("vHTLC with id %s already exists", vhtlcRow.ID)
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
			VhtlcID:      vhtlcRow.ID,
			SwapType:     int64(swap.Type),
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

func (r *swapRepository) Update(ctx context.Context, swap domain.Swap) error {
	existingSwap, err := r.Get(ctx, swap.Id)
	if err != nil {
		return fmt.Errorf("failed to get swap %s: %w", swap.Id, err)
	}

	if existingSwap == nil {
		return fmt.Errorf("existing swap %s does not exist", swap.Id)
	}

	if swap.Status != 0 {
		existingSwap.Status = swap.Status
	}

	if swap.RedeemTxId != "" {
		existingSwap.RedeemTxId = swap.RedeemTxId
	}

	err = r.querier.UpdateSwap(ctx, queries.UpdateSwapParams{
		Status:     int64(existingSwap.Status),
		RedeemTxID: existingSwap.RedeemTxId,
		ID:         swap.Id,
	})
	if err != nil {
		return fmt.Errorf("failed to update swap status: %w", err)
	}
	return nil
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

func toSwap(swapRow queries.Swap, vhtlcRow queries.Vhtlc) (*domain.Swap, error) {
	vhtlc, err := toVhtlc(vhtlcRow)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vhtlc opts: %w", err)
	}

	return &domain.Swap{
		Id:          swapRow.ID,
		Amount:      uint64(swapRow.Amount),
		Timestamp:   swapRow.Timestamp,
		To:          boltz.Currency(swapRow.ToCurrency),
		From:        boltz.Currency(swapRow.FromCurrency),
		Status:      domain.SwapStatus(swapRow.Status),
		Type:        domain.SwapType(swapRow.SwapType),
		Invoice:     swapRow.Invoice,
		FundingTxId: swapRow.FundingTxID,
		RedeemTxId:  swapRow.RedeemTxID,
		Vhtlc:       vhtlc,
	}, nil
}
