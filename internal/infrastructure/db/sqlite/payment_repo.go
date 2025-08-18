package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
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
	return r.querier.CreatePayment(ctx, queries.CreatePaymentParams{
		ID:          payment.Id,
		Amount:      int64(payment.Amount),
		Timestamp:   payment.Timestamp,
		Status:      int64(payment.Status),
		PaymentType: int64(payment.Type),
		Invoice:     payment.Invoice,
		TxID:        payment.TxId,
	})
}

func (r *paymentRepository) Update(ctx context.Context, payment domain.Payment) error {
	err := r.querier.UpdatePaymentStatus(ctx, queries.UpdatePaymentStatusParams{
		Status: int64(payment.Status),
		ID:     payment.Id,
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

	return toPayment(row)
}

func (r *paymentRepository) GetAll(ctx context.Context) ([]domain.Payment, error) {
	rows, err := r.querier.ListPayments(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.Payment
	for _, row := range rows {
		payment, err := toPayment(row)
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

func toPayment(payment queries.Payment) (*domain.Payment, error) {
	return &domain.Payment{
		Id:        payment.ID,
		Amount:    uint64(payment.Amount),
		Timestamp: payment.Timestamp,
		Status:    domain.PaymentStatus(payment.Status),
		Invoice:   payment.Invoice,
		TxId:      payment.TxID,
		Type:      domain.PaymentType(payment.PaymentType),
	}, nil
}
