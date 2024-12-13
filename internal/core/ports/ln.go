package ports

type LnService interface {
	Connect(lndconnectUrl string) error
	Disconnect()
	GetInfo() (version string, pubkey string, err error)
	GetInvoice(value int, note string) (invoice string, preimageHash string, err error)
	PayInvoice(invoice string) error
	IsConnected() bool
}
