package lnd

import (
	"fmt"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type service struct {
	client   lnrpc.LightningClient
	conn     *grpc.ClientConn
	macaroon string
}

func NewService() ports.LnService {
	return &service{nil, nil, ""}
}

func (s *service) Connect(lndconnectUrl string) error {
	if len(lndconnectUrl) == 0 {
		return fmt.Errorf("empty lnurl")
	}

	client, conn, macaroon, err := getClient(lndconnectUrl)
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	info, err := client.GetInfo(getCtx(macaroon), &lnrpc.GetInfoRequest{})
	if err != nil {
		return fmt.Errorf("unable to get info: %v", err)
	}

	if len(info.GetVersion()) == 0 {
		return fmt.Errorf("something went wrong, version is empty")
	}

	if len(info.GetIdentityPubkey()) == 0 {
		return fmt.Errorf("something went wrong, pubkey is empty")
	}

	s.client = client
	s.conn = conn
	s.macaroon = macaroon

	logrus.Infof("connected to LND version %s with pubkey %s", info.GetVersion(), info.GetIdentityPubkey())

	return nil
}

func (s *service) Disconnect() {
	s.conn.Close()
	s.client = nil
}

func (s *service) GetInfo() (version string, pubkey string, err error) {
	if !s.IsConnected() {
		err = fmt.Errorf("lnd service not connected")
		return
	}

	info, err := s.client.GetInfo(getCtx(s.macaroon), &lnrpc.GetInfoRequest{})
	if err != nil {
		return
	}

	return info.Version, info.IdentityPubkey, nil
}

func (s *service) GetInvoice(value int, memo string) (invoice string, err error) {
	if !s.IsConnected() {
		err = fmt.Errorf("lnd service not connected")
		return
	}

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
