package badgerdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

const (
	paymentDir = "payment"
)

type paymentRepository struct {
	store *badgerhold.Store
}

func NewPaymentRepository(baseDir string, logger badger.Logger) (domain.PaymentRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, paymentDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open payment store: %s", err)
	}
	return &paymentRepository{store}, nil
}

func (r *paymentRepository) Update(ctx context.Context, payment domain.Payment) error {
	return r.store.Update(payment.Id, &payment)
}

func (r *paymentRepository) GetAll(ctx context.Context) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.store.Find(&payments, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all swaps: %w", err)
	}

	return payments, nil
}

func (r *paymentRepository) Get(ctx context.Context, paymentId string) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.store.Get(paymentId, &payment)
	if errors.Is(err, badgerhold.ErrNotFound) {
		return nil, fmt.Errorf("swap %s not found", paymentId)
	}

	return &payment, nil
}

// Add stores a new Swap in the database
func (r *paymentRepository) Add(ctx context.Context, payment domain.Payment) error {

	if err := r.store.Insert(payment.Id, payment); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("payment %s already exists", payment.Id)
		}
		return err
	}
	return nil
}

func (s *paymentRepository) Close() {
	// nolint:all
	s.store.Close()
}
