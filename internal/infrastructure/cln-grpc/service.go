package cln_grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/cln"
	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type service struct {
	client cln.NodeClient
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
		return err
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
	s.client = cln.NewNodeClient(conn)

	return nil
}

func (s *service) IsConnected() bool {
	return s.client != nil
}

func (s *service) GetInfo(ctx context.Context) (version string, pubkey string, err error) {
	resp, err := s.client.Getinfo(ctx, &cln.GetinfoRequest{})
	if err != nil {
		return "", "", err
	}

	return resp.Version, hex.EncodeToString(resp.Id), nil
}

func (s *service) GetInvoice(ctx context.Context, value uint64, note string) (invoice string, preimageHash string, err error) {
	request := &cln.InvoiceRequest{
		AmountMsat: &cln.AmountOrAny{
			Value: &cln.AmountOrAny_Amount{
				Amount: &cln.Amount{
					Msat: value * 1000,
				},
			},
		},
		Description: note,
		Label:       fmt.Sprint(time.Now().UTC().UnixMilli()),
	}
	resp, err := s.client.Invoice(ctx, request)
	if err != nil {
		return "", "", err
	}

	return resp.Bolt11, hex.EncodeToString(resp.GetPaymentHash()), nil
}

func (s *service) PayInvoice(ctx context.Context, invoice string) (preimage string, err error) {
	res, err := s.client.Pay(ctx, &cln.PayRequest{
		Bolt11: invoice,
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(res.GetPaymentPreimage()), nil
}

func (s *service) Disconnect() {
	s.conn.Close()
}
