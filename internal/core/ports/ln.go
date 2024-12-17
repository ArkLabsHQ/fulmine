package ports

import "context"

type LnService interface {
	Connect(ctx context.Context, lndconnectUrl string) error
	IsConnected() bool
	GetInfo(ctx context.Context) (version string, pubkey string, err error)
	GetInvoice(ctx context.Context, value uint64, note string) (invoice string, preimageHash string, err error)
	PayInvoice(ctx context.Context, invoice string) (preimage string, err error)
	Disconnect()
}
