package lnd

import (
	"fmt"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/sirupsen/logrus"
)

type service struct {
	client   lnrpc.LightningClient
	macaroon string
}

func NewService() ports.LnService {
	return &service{nil, ""}
}

func (s *service) Connect(lndconnectUrl string) error {
	if len(lndconnectUrl) == 0 {
		return fmt.Errorf("empty lnurl")
	}

	client, macaroon, err := getClient(lndconnectUrl)
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	s.client = client
	s.macaroon = macaroon

	info, err := s.GetInfo()
	if err != nil {
		return fmt.Errorf("unable to get info: %v", err)
	}

	if len(info.Version) == 0 {
		return fmt.Errorf("something went wrong, version is empty")
	}

	logrus.Infof("connected to LND version %s", info.Version)

	invoice, err := s.GetInvoice(21, "")
	if err != nil {
		return fmt.Errorf("unable to get invoice: %v", err)
	}

	logrus.Info(invoice.PaymentRequest)

	return nil
}

func (s *service) Disconnect() {
	s.client = nil
}

func (s *service) GetInfo() (*lnrpc.GetInfoResponse, error) {
	return s.client.GetInfo(getCtx(s.macaroon), &lnrpc.GetInfoRequest{})
}

func (s *service) GetInvoice(value int, memo string) (*lnrpc.AddInvoiceResponse, error) {
	invoiceRequest := &lnrpc.Invoice{
		Value: int64(value), // amount in satoshis
		Memo:  memo,         // optional memo
	}
	return s.client.AddInvoice(getCtx(s.macaroon), invoiceRequest)
}

func (s *service) IsConnected() bool {
	return s.client != nil
}
