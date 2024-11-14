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

	version, pubkey, err := s.GetInfo()
	if err != nil {
		return fmt.Errorf("unable to get info: %v", err)
	}

	if len(version) == 0 {
		return fmt.Errorf("something went wrong, version is empty")
	}

	if len(pubkey) == 0 {
		return fmt.Errorf("something went wrong, pubkey is empty")
	}

	logrus.Infof("connected to LND version %s with pubkey %s", version, pubkey)

	return nil
}

func (s *service) Disconnect() {
	s.client = nil
}

func (s *service) GetInfo() (version string, pubkey string, err error) {
	info, err := s.client.GetInfo(getCtx(s.macaroon), &lnrpc.GetInfoRequest{})
	if err != nil {
		return
	}

	return info.Version, info.IdentityPubkey, nil
}

func (s *service) GetInvoice(value int, memo string) (invoice string, err error) {
	invoiceRequest := &lnrpc.Invoice{
		Value: int64(value), // amount in satoshis
		Memo:  memo,         // optional memo
	}

	info, err := s.client.AddInvoice(getCtx(s.macaroon), invoiceRequest)
	if err != nil {
		return
	}

	return info.PaymentRequest, nil
}

func (s *service) IsConnected() bool {
	return s.client != nil
}
