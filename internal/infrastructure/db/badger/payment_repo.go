package badgerdb

import (
	"context"
	"encoding/hex"
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
	paymentData := toPaymentData(payment)
	return r.store.Update(payment.Id, paymentData)
}

func (r *paymentRepository) GetAll(ctx context.Context) ([]domain.Payment, error) {
	var paymentDataList []paymentData
	err := r.store.Find(&paymentDataList, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all payments: %w", err)
	}

	var payments []domain.Payment
	for _, s := range paymentDataList {
		payment, err := s.toPayment()
		if err != nil {
			return nil, fmt.Errorf("failed to convert data to payment: %w", err)
		}

		payments = append(payments, *payment)
	}
	return payments, nil
}

func (r *paymentRepository) Get(ctx context.Context, paymentId string) (*domain.Payment, error) {
	var paymentData paymentData
	err := r.store.Get(paymentId, &paymentData)
	if errors.Is(err, badgerhold.ErrNotFound) {
		return nil, fmt.Errorf("payment %s not found", paymentId)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	payment, err := paymentData.toPayment()
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to payment: %w", err)
	}

	return payment, nil
}

// Add stores a new Payment in the database
func (r *paymentRepository) Add(ctx context.Context, payment domain.Payment) error {
	paymentData := toPaymentData(payment)

	if err := r.store.Insert(payment.Id, paymentData); err != nil {
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

type paymentData struct {
	Id            string
	Amount        uint64
	Timestamp     int64
	Status        domain.PaymentStatus
	Type          domain.PaymentType
	Invoice       string
	TxId          string
	ReclaimedTxId string
	Opts          vhtlcData
}

func toPaymentData(payment domain.Payment) paymentData {
	vhtlcOpts := payment.Opts

	vhtlcData := vhtlcData{
		PreimageHash:                         hex.EncodeToString(vhtlcOpts.PreimageHash),
		Sender:                               hex.EncodeToString(vhtlcOpts.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(vhtlcOpts.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(vhtlcOpts.Server.SerializeCompressed()),
		RefundLocktime:                       vhtlcOpts.RefundLocktime,
		UnilateralClaimDelay:                 vhtlcOpts.UnilateralClaimDelay,
		UnilateralRefundDelay:                vhtlcOpts.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: vhtlcOpts.UnilateralRefundWithoutReceiverDelay,
	}

	return paymentData{
		Id:        payment.Id,
		Amount:    payment.Amount,
		Timestamp: payment.Timestamp,

		Status:        payment.Status,
		Invoice:       payment.Invoice,
		Opts:          vhtlcData,
		ReclaimedTxId: payment.ReclaimedTxId,
		TxId:          payment.TxId,
		Type:          payment.Type,
	}
}

func (s *paymentData) toPayment() (*domain.Payment, error) {
	vhtlcOps, err := s.Opts.toOpts()
	if err != nil {
		return nil, err
	}

	return &domain.Payment{
		Id:            s.Id,
		Amount:        s.Amount,
		Timestamp:     s.Timestamp,
		Type:          s.Type,
		Status:        s.Status,
		Invoice:       s.Invoice,
		Opts:          *vhtlcOps,
		ReclaimedTxId: s.ReclaimedTxId,
		TxId:          s.TxId,
	}, nil
}
