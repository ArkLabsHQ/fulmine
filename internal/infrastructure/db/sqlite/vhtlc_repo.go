package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type vhtlcRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewVHTLCRepository(db *sql.DB) (domain.VHTLCRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open vhtlc repository: db is nil")
	}
	return &vhtlcRepository{db: db, querier: queries.New(db)}, nil
}

func (r *vhtlcRepository) Add(ctx context.Context, vhtlc domain.Vhtlc) error {
	if err := r.querier.InsertVHTLC(ctx, queries.InsertVHTLCParams{
		ID:     vhtlc.Id,
		Script: vhtlc.Script,
	}); err != nil {
		if sqlErr, ok := err.(*sqlite.Error); ok {
			if sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
				return fmt.Errorf("vHTLC with ID %s already exists", vhtlc.Id)
			}
		}
		return err
	}
	return nil
}

func (r *vhtlcRepository) Get(ctx context.Context, id string) (*domain.Vhtlc, error) {
	row, err := r.querier.GetVHTLC(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("vHTLC with id %s not found", id)

		}
		return nil, err
	}

	return &domain.Vhtlc{
		Id:     row.ID,
		Script: row.Script,
	}, nil
}

func (r *vhtlcRepository) GetByIds(ctx context.Context, ids []string) ([]domain.Vhtlc, error) {
	rows, err := r.querier.ListVHTLCsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Vhtlc, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Vhtlc{
			Id:     row.ID,
			Script: row.Script,
		})
	}
	return out, nil
}

func (r *vhtlcRepository) GetAll(ctx context.Context) ([]domain.Vhtlc, error) {
	rows, err := r.querier.ListVHTLC(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Vhtlc, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Vhtlc{
			Id:     row.ID,
			Script: row.Script,
		})
	}
	return out, nil
}

func (r *vhtlcRepository) HasLegacy(ctx context.Context) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vhtlc_legacy'`,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *vhtlcRepository) GetLegacy(ctx context.Context) ([]domain.LegacyVhtlc, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT preimage_hash, sender, receiver, server, refund_locktime,
			unilateral_claim_delay_type, unilateral_claim_delay_value,
			unilateral_refund_delay_type, unilateral_refund_delay_value,
			unilateral_refund_without_receiver_delay_type,
			unilateral_refund_without_receiver_delay_value
		FROM vhtlc_legacy`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.LegacyVhtlc, 0)
	for rows.Next() {
		var v domain.LegacyVhtlc
		if err := rows.Scan(
			&v.PreimageHash, &v.Sender, &v.Receiver, &v.Server, &v.RefundLocktime,
			&v.UnilateralClaimDelayType, &v.UnilateralClaimDelayValue,
			&v.UnilateralRefundDelayType, &v.UnilateralRefundDelayValue,
			&v.UnilateralRefundWithoutReceiverDelayType,
			&v.UnilateralRefundWithoutReceiverDelayValue,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *vhtlcRepository) DropLegacy(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DROP TABLE IF EXISTS vhtlc_legacy`)
	return err
}

func (r *vhtlcRepository) Close() {
	if r.db != nil {
		r.db.Close()
	}
}
