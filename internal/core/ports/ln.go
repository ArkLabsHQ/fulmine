package ports

import "context"

type LnService interface {
	Connect(ctx context.Context, lndConnectUrl string) error
	IsConnected() bool
	GetInfo(ctx context.Context) (version string, pubkey string, err error)
	GetInvoice(ctx context.Context, value uint64, note, preimage string) (invoice string, preimageHash string, err error)
	IsInvoiceSettled(ctx context.Context, invoice string) (ok bool, err error)
	PayInvoice(ctx context.Context, invoice string) (preimage string, err error)
	Disconnect()
	GetBalance(ctx context.Context) (balance uint64, err error)
}
