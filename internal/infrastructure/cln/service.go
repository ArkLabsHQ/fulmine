package cln

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"

	cln_pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/cln"
	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/btcsuite/btcd/btcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type service struct {
	client cln_pb.NodeClient
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
	})

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}

	s.conn = conn
	s.client = cln_pb.NewNodeClient(conn)

	return nil
}

func (s *service) IsConnected() bool {
	return s.client != nil
}

func (s *service) GetInfo(ctx context.Context) (version string, pubkey string, err error) {
	resp, err := s.client.Getinfo(ctx, &cln_pb.GetinfoRequest{})
	if err != nil {
		return "", "", err
	}

	return resp.Version, hex.EncodeToString(resp.Id), nil
}

func (s *service) GetInvoice(
	ctx context.Context, value uint64, note, preimage string,
) (invoice string, preimageHash string, err error) {
	request := &cln_pb.InvoiceRequest{
		AmountMsat: &cln_pb.AmountOrAny{
			Value: &cln_pb.AmountOrAny_Amount{
				Amount: &cln_pb.Amount{
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

	return resp.Bolt11, hex.EncodeToString(btcutil.Hash160(resp.GetPaymentSecret())), nil
}

func (s *service) PayInvoice(ctx context.Context, invoice string) (preimage string, err error) {
	res, err := s.client.Pay(ctx, &cln_pb.PayRequest{
		Bolt11: invoice,
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(res.GetPaymentPreimage()), nil
}

func (s *service) Disconnect() {
	s.conn.Close()
	s.client = nil
	s.conn = nil
}

func (s *service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	// TODO: get the hash of the invoice
	invoiceResp, err := s.client.ListInvoices(ctx, &cln_pb.ListinvoicesRequest{
		PaymentHash: []byte(invoice),
	})
	if err != nil {
		return false, err
	}
	if len(invoiceResp.Invoices) == 0 {
		return false, fmt.Errorf("invoice not found")
	}
	return invoiceResp.Invoices[0].Status == 1, nil // TODO
}
