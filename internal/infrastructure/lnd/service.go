package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	ErrServiceNotConnected = fmt.Errorf("lnd service not connected")
)

type service struct {
	client       lnrpc.LightningClient
	routerClient routerrpc.RouterClient
	conn         *grpc.ClientConn
	lnConnectUrl string
	macaroon     string
}

func NewService() ports.LnService {
	return &service{nil, nil, nil, "", ""}
}

func (s *service) Connect(ctx context.Context, opts *domain.LnConnectionOpts, network string) (err error) {
	var conn *grpc.ClientConn
	var macaroon string
	var lnConnectUrl string

	if strings.HasPrefix(opts.LnUrl, "lndconnect:") {
		conn, macaroon, err = deriveLndConnFromUrl(opts.LnUrl)
		lnConnectUrl = opts.LnUrl

	} else {
		conn, macaroon, lnConnectUrl, err = deriveLndConnFromPath(opts.LnDatadir, opts.LnUrl, network)
	}

	if err != nil {
		return fmt.Errorf("error deriving LND connection: %w", err)
	}

	client := lnrpc.NewLightningClient(conn)
	routerClient := routerrpc.NewRouterClient(conn)

	ctx = getCtx(ctx, macaroon)
	info, err := client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return fmt.Errorf("failed to connect to LND: %v", err)
	}

	if len(info.GetVersion()) == 0 {
		return fmt.Errorf("something went wrong, version is empty")
	}

	if len(info.GetIdentityPubkey()) == 0 {
		return fmt.Errorf("something went wrong, pubkey is empty")
	}

	s.client = client
	s.routerClient = routerClient
	s.conn = conn
	s.macaroon = macaroon
	s.lnConnectUrl = lnConnectUrl

	log.Infof("connected to LND version %s with pubkey %s", info.GetVersion(), info.GetIdentityPubkey())

	return nil
}

func (s *service) Disconnect() {
	// nolint:all
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

func (s *service) GetLnConnectUrl() string {
	return s.lnConnectUrl
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

func (s *service) DecodeInvoice(ctx context.Context, invoice string) (value uint64, preimageHash []byte, err error) {
	if !s.IsConnected() {
		return 0, nil, ErrServiceNotConnected
	}

	decodeResp, err := s.client.DecodePayReq(getCtx(ctx, s.macaroon), &lnrpc.PayReqString{PayReq: invoice})
	if err != nil {
		return 0, nil, err
	}

	preimageHash, err = hex.DecodeString(decodeResp.PaymentHash)
	if err != nil {
		return 0, nil, err
	}

	return uint64(decodeResp.NumSatoshis), preimageHash, nil
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

	sendRequest := &routerrpc.SendPaymentRequest{
		PaymentRequest: invoice,
		TimeoutSeconds: 120,
	}
	stream, err := s.routerClient.SendPaymentV2(ctx, sendRequest)
	if err != nil {
		return "", err
	}

	var preimage string
	var success bool
	for {
		update, err := stream.Recv()
		if err == io.EOF {
			log.Println("stream closed")
			break
		}
		if err != nil {
			log.Fatalf("stream error: %v", err)
		}

		switch update.GetStatus() {
		case lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_SUCCEEDED):
			preimage = update.GetPaymentPreimage()
			success = true
		case lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_FAILED_ERROR),
			lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_FAILED_INCORRECT_PAYMENT_DETAILS),
			lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_FAILED_INSUFFICIENT_BALANCE),
			lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_FAILED_NO_ROUTE),
			lnrpc.Payment_PaymentStatus(routerrpc.PaymentState_FAILED_TIMEOUT):
			return "", fmt.Errorf("%s", update.GetFailureReason().String())
		}

		if success {
			break
		}
	}
	return preimage, nil
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
