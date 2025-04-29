package cln

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"

	clnpb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/cln"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/lightningnetwork/lnd/input"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type service struct {
	client clnpb.NodeClient
	conn   *grpc.ClientConn
}

func NewService() ports.LnService {
	return &service{nil, nil}
}

func (s *service) Connect(ctx context.Context, clnConnectUrl string) error {
	rootCert, privateKey, certChain, host, err := decodeClnConnectUrl(clnConnectUrl)
	if err != nil {
		return err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(rootCert)) {
		return fmt.Errorf("could not parse root certificate")
	}

	cert, err := tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	if err != nil {
		return fmt.Errorf("error with X509KeyPair, %s", err)
	}

	creds := credentials.NewTLS(&tls.Config{
		ServerName:   "cln",
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}

	s.conn = conn
	s.client = clnpb.NewNodeClient(conn)

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
	total := uint64(0)
	resp, err := s.client.ListFunds(ctx, &clnpb.ListfundsRequest{})
	if err != nil {
		return 0, err
	}
	for _, channel := range resp.GetChannels() {
		total += channel.GetOurAmountMsat().Msat
	}
	return total, nil
}
