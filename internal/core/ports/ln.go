package ports

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
)

type LnService interface {
	Connect(ctx context.Context, opts *domain.LnConnectionOpts, network string) error
	IsConnected() bool
	GetInfo(ctx context.Context) (version string, pubkey string, err error)
	GetLnConnectUrl() string
	GetInvoice(ctx context.Context, value uint64, note, preimage string) (invoice string, preimageHash string, err error)
	DecodeInvoice(ctx context.Context, invoice string) (value uint64, preimageHash []byte, err error)
	IsInvoiceSettled(ctx context.Context, invoice string) (ok bool, err error)
	PayInvoice(ctx context.Context, invoice string) (preimage string, err error)
	Disconnect()
	GetBalance(ctx context.Context) (balance uint64, err error)
}
