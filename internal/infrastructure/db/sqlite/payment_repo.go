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

type paymentRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewPaymentRepository(db *sql.DB) (domain.PaymentRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open vtxo rollover repository: db is nil")
	}

	return &paymentRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (r *paymentRepository) Add(ctx context.Context, payment domain.Payment) error {
	txBody := func(querierWithTx *queries.Queries) error {
		optsParams := toOptParams(payment.Opts)
		preimageHash := optsParams.PreimageHash

		if err := querierWithTx.InsertVHTLC(ctx, optsParams); err != nil {
			if sqlErr, ok := err.(*sqlite.Error); ok {
				if sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
					return fmt.Errorf("vHTLC with preimage hash %s already exists", optsParams.PreimageHash)
				}
			}
			return fmt.Errorf("failed to insert vhtlc: %s", err)
		}

		if err := querierWithTx.CreatePayment(ctx, queries.CreatePaymentParams{
			ID:          payment.Id,
			Amount:      int64(payment.Amount),
			Timestamp:   payment.Timestamp,
			Status:      int64(payment.Status),
			PaymentType: int64(payment.Type),
			Invoice:     payment.Invoice,
			TxID:        payment.TxId,
			VhtlcID:     preimageHash,
		}); err != nil {
			return fmt.Errorf("failed to insert swap: %s", err)
		}
		return nil
	}
	return execTx(ctx, r.db, txBody)
}

func (r *paymentRepository) Update(ctx context.Context, payment domain.Payment) error {
	existingPayment, err := r.Get(ctx, payment.Id)
	if err != nil {
		return fmt.Errorf("failed to get payment %s: %w", payment.Id, err)
	}
	if existingPayment == nil {
		return fmt.Errorf("payment %s does not exist", payment.Id)
	}

	if payment.Status != 0 {
		existingPayment.Status = payment.Status
	}

	if payment.ReclaimedTxId != "" {
		existingPayment.ReclaimedTxId = payment.ReclaimedTxId
	}

	err = r.querier.UpdatePayment(ctx, queries.UpdatePaymentParams{
		Status:      int64(existingPayment.Status),
		ReclaimTxID: sql.NullString{String: existingPayment.ReclaimedTxId, Valid: true},
		ID:          payment.Id,
	})

	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}
	return nil
}

func (r *paymentRepository) Get(ctx context.Context, paymentId string) (*domain.Payment, error) {
	row, err := r.querier.GetPayment(ctx, paymentId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("swap %s not found", paymentId)
		}
		return nil, err
	}

	return toPayment(row.Payment, row.Vhtlc)
}

func (r *paymentRepository) GetAll(ctx context.Context) ([]domain.Payment, error) {
	rows, err := r.querier.ListPayments(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.Payment
	for _, row := range rows {
		payment, err := toPayment(row.Payment, row.Vhtlc)
		if err != nil {
			return nil, err
		}
		results = append(results, *payment)
	}
	return results, nil
}

func (r *paymentRepository) Close() {
	// nolint
	r.db.Close()
}

func toPayment(payment queries.Payment, vhtlc queries.Vhtlc) (*domain.Payment, error) {
	vhtlcOpts, err := toOpts(vhtlc)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vhtlc opts: %w", err)
	}

	return &domain.Payment{
		Id:            payment.ID,
		Amount:        uint64(payment.Amount),
		Timestamp:     payment.Timestamp,
		Status:        domain.PaymentStatus(payment.Status),
		Invoice:       payment.Invoice,
		TxId:          payment.TxID,
		Type:          domain.PaymentType(payment.PaymentType),
		ReclaimedTxId: payment.ReclaimTxID.String,
		Opts:          *vhtlcOpts,
	}, nil
}
