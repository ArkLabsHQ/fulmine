package domain

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
)

type PaymentStatus int

const (
	PaymentPending PaymentStatus = iota
	PaymentRefunding
	PaymentFailed
	PaymentSuccess
)

type PaymentType int

const (
	PaymentReceive PaymentType = iota
	PaymentSend
)

type Payment struct {
	Id        string
	Amount    uint64
	Timestamp int64

	Status        PaymentStatus
	Type          PaymentType
	Invoice       string
	TxId          string
	ReclaimedTxId string
	Opts          vhtlc.Opts
}

// PaymentRepository stores the Payment initiated by the wallet
type PaymentRepository interface {
	GetAll(ctx context.Context) ([]Payment, error)
	Get(ctx context.Context, paymentId string) (*Payment, error)
	Add(ctx context.Context, payment Payment) error
	Update(ctx context.Context, payment Payment) error
	Close()
}
