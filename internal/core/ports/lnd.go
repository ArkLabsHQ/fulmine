package ports

import "github.com/lightningnetwork/lnd/lnrpc"

type LndService interface {
	Connect(lndconnectUrl string) error
	Disconnect()
	GetInfo() (*lnrpc.GetInfoResponse, error)
	GetInvoice(value int, note string) (*lnrpc.AddInvoiceResponse, error)
	IsConnected() bool
}
