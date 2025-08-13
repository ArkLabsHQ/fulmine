package domain

import (
	"context"
)

type PaymentStatus int

const (
	PaymentPending PaymentStatus = iota
	PaymentFailed
	PaymentSuccess
)

type PaymentType int

const (
	Receive PaymentType = iota
	Pay
)

type Payment struct {
	Id        string
	Amount    uint64
	Timestamp int64

	Status  PaymentStatus
	Type    PaymentType
	Invoice string
	TxId    string
}

// PaymentRepository stores the Payment initiated by the wallet
type PaymentRepository interface {
	GetAll(ctx context.Context) ([]Payment, error)
	Get(ctx context.Context, paymentId string) (*Payment, error)
	Add(ctx context.Context, payment Payment) error
	Close()
}
