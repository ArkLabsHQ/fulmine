package cln_grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/cln"
	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const serverName = "cln"

type service struct {
	rootCert   string
	privateKey string
	certChain  string

	client cln.NodeClient
	conn   *grpc.ClientConn
}

func NewService(rootCert, privateKey, certChain string) (ports.LnService, error) {
	if _, err := os.Stat(rootCert); err != nil {
		return nil, fmt.Errorf("root cert file at path %s does not exist: %w", rootCert, err)
	}
	if _, err := os.Stat(privateKey); err != nil {
		return nil, fmt.Errorf("private key file at path %s does not exist: %w", privateKey, err)
	}
	if _, err := os.Stat(certChain); err != nil {
		return nil, fmt.Errorf("cert chain file at path %s does not exist: %w", certChain, err)
	}
	return &service{
		rootCert:   rootCert,
		privateKey: privateKey,
		certChain:  certChain,
	}, nil
}

func (s *service) Connect(ctx context.Context, serverUrl string) error {
	caFile, err := os.ReadFile(s.rootCert)
	if err != nil {
		return err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caFile) {
		return fmt.Errorf("could not parse root certificate")
	}

	cert, err := tls.LoadX509KeyPair(s.certChain, s.privateKey)
	if err != nil {
		return err
	}

	creds := credentials.NewTLS(&tls.Config{
		ServerName:   serverName,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
	})
	conn, err := grpc.NewClient(serverUrl, grpc.WithTransportCredentials(creds))
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
