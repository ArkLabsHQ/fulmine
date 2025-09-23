package cln

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	clnpb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/cln"
	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/lightningnetwork/lnd/input"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type service struct {
	client       clnpb.NodeClient
	conn         *grpc.ClientConn
	lnConnectUrl string
}

func NewService() ports.LnService {
	return &service{nil, nil, ""}
}

func (s *service) Connect(ctx context.Context, opts *domain.LnConnectionOpts, network string) (err error) {
	var conn *grpc.ClientConn
	var lnConnectUrl string

	if strings.HasPrefix(opts.LnUrl, "clnconnect:") {
		conn, err = deriveClnConnFromUrl(opts.LnUrl)
		lnConnectUrl = opts.LnUrl

	} else {
		conn, lnConnectUrl, err = deriveClnConnFromPath(opts.LnDatadir, opts.LnUrl, network)
	}

	if err != nil {
		return fmt.Errorf("error deriving CLN connection : %w", err)
	}

	s.conn = conn
	s.client = clnpb.NewNodeClient(conn)
	s.lnConnectUrl = lnConnectUrl

	resp, err := s.client.Getinfo(ctx, &clnpb.GetinfoRequest{})
	if err != nil {
		return fmt.Errorf("failed to connect to CLN: %w", err)
	}
	if resp.GetVersion() == "" {
		return fmt.Errorf("something went wrong, version is empty")
	}
	if len(resp.GetId()) <= 0 {
		return fmt.Errorf("something went wrong, pubkey is empty")
	}

	log.Infof("connected to CLN version %s with pubkey %x", resp.GetVersion(), resp.GetId())

	return nil
}

func (s *service) IsConnected() bool {
	return s.client != nil
}

func (s *service) GetInfo(ctx context.Context) (version string, pubkey string, err error) {
	resp, err := s.client.Getinfo(ctx, &clnpb.GetinfoRequest{})
	if err != nil {
		return "", "", err
	}

	return resp.Version, hex.EncodeToString(resp.Id), nil
}

func (s *service) GetLnConnectUrl() string {
	return s.lnConnectUrl
}

func (s *service) GetInvoice(
	ctx context.Context, value uint64, note, preimage string,
) (string, string, error) {
	request := &clnpb.InvoiceRequest{
		AmountMsat: &clnpb.AmountOrAny{
			Value: &clnpb.AmountOrAny_Amount{
				Amount: &clnpb.Amount{
					Msat: value * 1000,
				},
			},
		},
		Description: note,
		Label:       fmt.Sprint(time.Now().UTC().UnixMilli()),
	}

	if len(preimage) > 0 {
		request.Preimage = []byte(preimage)
	}

	resp, err := s.client.Invoice(ctx, request)
	if err != nil {
		return "", "", err
	}

	preimageHash := hex.EncodeToString(input.Ripemd160H(resp.GetPaymentHash()))
	return resp.Bolt11, preimageHash, nil
}

func (s *service) DecodeInvoice(ctx context.Context, invoice string) (value uint64, preimageHash []byte, err error) {
	decodeResp, err := s.client.Decode(ctx, &clnpb.DecodeRequest{String_: invoice})
	if err != nil {
		return 0, nil, err
	}

	return decodeResp.AmountMsat.Msat / 1000, decodeResp.PaymentHash, nil
}

func (s *service) PayInvoice(ctx context.Context, invoice string) (preimage string, err error) {
	res, err := s.client.Pay(ctx, &clnpb.PayRequest{
		Bolt11: invoice,
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(res.GetPaymentPreimage()), nil
}

func (s *service) Disconnect() {
	// nolint:all
	s.conn.Close()
	s.client = nil
	s.conn = nil
}

func (s *service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	decodeResp, err := s.client.Decode(ctx, &clnpb.DecodeRequest{String_: invoice})
	if err != nil {
		return false, err
	}
	invoiceResp, err := s.client.ListInvoices(ctx, &clnpb.ListinvoicesRequest{
		PaymentHash: decodeResp.GetPaymentHash(),
	})
	if err != nil {
		return false, err
	}
	if len(invoiceResp.Invoices) == 0 {
		return false, fmt.Errorf("invoice not found")
	}
	return invoiceResp.Invoices[0].Status == clnpb.ListinvoicesInvoices_PAID, nil // TODO
}

func (s *service) GetBalance(ctx context.Context) (uint64, error) {
	balance := uint64(0)
	resp, err := s.client.ListFunds(ctx, &clnpb.ListfundsRequest{})
	if err != nil {
		return 0, err
	}
	for _, channel := range resp.GetChannels() {
		balance += channel.GetOurAmountMsat().Msat
	}

	return balance, nil
}
