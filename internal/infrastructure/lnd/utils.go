package lnd

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// decodes lndconnect:// url into cert, macaroon and host
func decodeLNDConnectUrl(lndconnectUrl string) (cp *x509.CertPool, macaroon string, host string, err error) {
	u, err := url.Parse(lndconnectUrl)
	if err != nil {
		return
	}

	cert := toBase64(u.Query().Get("cert"))

	cp = x509.NewCertPool()
	certPEM := "-----BEGIN CERTIFICATE-----\n" + cert + "\n-----END CERTIFICATE-----"
	if !cp.AppendCertsFromPEM([]byte(certPEM)) {
		err = fmt.Errorf("credentials: failed to append certificates")
		return
	}

	macaroonBase64 := toBase64(u.Query().Get("macaroon"))
	decodedBytes, err := base64.StdEncoding.DecodeString(macaroonBase64)
	if err != nil {
		err = fmt.Errorf("failed to decode base64: %v", err)
		return
	}
	macaroon = hex.EncodeToString(decodedBytes)

	host = u.Host

	return
}

func getClient(lndconnectUrl string) (client lnrpc.LightningClient, conn *grpc.ClientConn, macaroon string, err error) {
	// decode url
	cert, macaroon, host, err := decodeLNDConnectUrl(lndconnectUrl)
	if err != nil {
		return
	}
	// check credentials (only cert, not macaroon)
	creds := credentials.NewClientTLSFromCert(cert, "")
	conn, err = grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return
	}

	return lnrpc.NewLightningClient(conn), conn, macaroon, nil
}

func getCtx(macaroon string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "macaroon", macaroon)
}

// padBase64 adds '=' characters to the end of the input
// string until its length is a multiple of 4.
func padBase64(input string) string {
	length := len(input)
	padding := (4 - (length % 4)) % 4
	for i := 0; i < padding; i++ {
		input += "="
	}
	return input
}

// from url safe string to base64
func toBase64(input string) string {
	input = padBase64(input)
	input = strings.ReplaceAll(input, "-", "+")
	input = strings.ReplaceAll(input, "_", "/")
	return input
}
