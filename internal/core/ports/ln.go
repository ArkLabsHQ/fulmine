package ports

type LnService interface {
	Connect(lndconnectUrl string) error
	Disconnect()
	GetInfo() (version string, pubkey string, err error)
	GetInvoice(value int, note string) (invoice string, err error)
	IsConnected() bool
}
