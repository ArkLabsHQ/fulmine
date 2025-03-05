package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnrpc"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	ErrServiceNotConnected = fmt.Errorf("lnd service not connected")
)

type service struct {
	client   lnrpc.LightningClient
	conn     *grpc.ClientConn
	macaroon string
}

func NewService() ports.LnService {
	return &service{nil, nil, ""}
}

func (s *service) Connect(ctx context.Context, lndConnectUrl string) error {
	if len(lndConnectUrl) == 0 {
		return fmt.Errorf("empty lnurl")
	}

	client, conn, macaroon, err := getClient(lndConnectUrl)
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	ctx = getCtx(ctx, macaroon)
	info, err := client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
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

	log.Infof("connected to LND version %s with pubkey %s", info.GetVersion(), info.GetIdentityPubkey())

	return nil
}

func (s *service) Disconnect() {
	s.conn.Close()
	s.client = nil
	s.conn = nil
}

func (s *service) GetInfo(ctx context.Context) (version, pubkey string, err error) {
	if !s.IsConnected() {
		err = ErrServiceNotConnected
		return
	}

	ctx = getCtx(ctx, s.macaroon)
	info, err := s.client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return
	}

	return info.Version, info.IdentityPubkey, nil
}

func (s *service) GetInvoice(
	ctx context.Context, value uint64, memo, preimage string,
) (string, string, error) {
	if !s.IsConnected() {
		return "", "", ErrServiceNotConnected
	}

	ctx = getCtx(ctx, s.macaroon)
	invoiceRequest := &lnrpc.Invoice{
		// #nosec
		Value: int64(value), // amount in satoshis
		Memo:  memo,         // optional memo
	}

	if len(preimage) > 0 {
		invoiceRequest.RPreimage = []byte(preimage)
	}

	info, err := s.client.AddInvoice(ctx, invoiceRequest)
	if err != nil {
		return "", "", err
	}

	preimageHash := hex.EncodeToString(input.Ripemd160H(info.GetRHash()))
	return info.PaymentRequest, preimageHash, nil
}

func (s *service) IsConnected() bool {
	return s.client != nil
}

func (s *service) PayInvoice(
	ctx context.Context, invoice string,
) (string, error) {
	if !s.IsConnected() {
		return "", ErrServiceNotConnected
	}

	invoice = strings.TrimPrefix(strings.ToLower(invoice), "lightning=")

	// validate invoice
	ctx = getCtx(ctx, s.macaroon)
	decodeRequest := &lnrpc.PayReqString{PayReq: invoice}
	if _, err := s.client.DecodePayReq(ctx, decodeRequest); err != nil {
		return "", fmt.Errorf("invalid invoice %s : %s", err, invoice)
	}

	sendRequest := &lnrpc.SendRequest{PaymentRequest: invoice}
	response, err := s.client.SendPaymentSync(ctx, sendRequest)
	if err != nil {
		return "", err
	}

	if err := response.GetPaymentError(); err != "" {
		return "", fmt.Errorf("%s", err)
	}

	return hex.EncodeToString(response.GetPaymentPreimage()), nil
}

func (s *service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	ctx = getCtx(ctx, s.macaroon)
	decodeResp, err := s.client.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: invoice})
	if err != nil {
		return false, err
	}
	invoiceResp, err := s.client.LookupInvoice(ctx, &lnrpc.PaymentHash{
		RHashStr: decodeResp.GetPaymentHash(),
	})
	if err != nil {
		return false, err
	}
	return invoiceResp.State == lnrpc.Invoice_SETTLED, nil
}

func (s *service) GetBalance(ctx context.Context) (uint64, error) {
	ctx = getCtx(ctx, s.macaroon)
	resp, err := s.client.ChannelBalance(ctx, &lnrpc.ChannelBalanceRequest{})
	if err != nil {
		return 0, err
	}
	return uint64(resp.GetLocalBalance().Msat), nil
}
