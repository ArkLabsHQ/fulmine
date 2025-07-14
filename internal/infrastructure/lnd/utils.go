package lnd

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// decodes lndconnect:// url into cert, macaroon and host
func decodeLNDConnectUrl(lndConnectUrl string) (cp *x509.CertPool, macaroon string, host string, err error) {
	u, err := url.Parse(lndConnectUrl)
	if err != nil {
		return
	}

	cert := toBase64(u.Query().Get("cert"))
	if cert != "" {
		cp = x509.NewCertPool()
		certPEM := "-----BEGIN CERTIFICATE-----\n" + cert + "\n-----END CERTIFICATE-----"
		if !cp.AppendCertsFromPEM([]byte(certPEM)) {
			return nil, "", "", fmt.Errorf("credentials: failed to append certificates")
		}
	}

	macaroonBase64 := toBase64(u.Query().Get("macaroon"))
	if macaroonBase64 != "" {
		decodedBytes, err := base64.StdEncoding.DecodeString(macaroonBase64)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to decode base64: %v", err)
		}
		macaroon = hex.EncodeToString(decodedBytes)
	}

	host = u.Host

	return
}

func getCtx(ctx context.Context, macaroon string) context.Context {
	if macaroon == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "macaroon", macaroon)
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

func deriveLndConnFromUrl(lndConnectUrl string) (conn *grpc.ClientConn, macaroon string, err error) {
	cert, macaroon, host, err := decodeLNDConnectUrl(lndConnectUrl)
	if err != nil {
		return
	}
	// check credentials (only cert, not macaroon)
	creds := insecure.NewCredentials()
	if cert != nil {
		creds = credentials.NewClientTLSFromCert(cert, "")
	}
	conn, err = grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return
	}

	return conn, macaroon, nil
}

func deriveLndConnFromPath(dataDir, host, network string) (conn *grpc.ClientConn, macaroon, lnUrl string, err error) {
	macQueryString := ""
	lndNetwork := deriveLndNetwork(network)
	macPath := filepath.Join(dataDir, "data", "chain", "bitcoin", lndNetwork, "admin.macaroon")
	if _, err := os.Stat(macPath); err == nil {
		macBytes, err := os.ReadFile(macPath)
		if err != nil {
			return nil, "", "", err
		}

		macaroon = hex.EncodeToString(macBytes)
		macQueryString = "?macaroon=" + macaroon
	}

	tlsQueryString := ""
	tlsPath := filepath.Join(dataDir, "tls.cert")
	creds := insecure.NewCredentials()
	if _, err := os.Stat(tlsPath); err == nil {
		tlsBytes, err := os.ReadFile(tlsPath)
		if err != nil {
			return nil, "", "", err
		}

		block, _ := pem.Decode(tlsBytes)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, "", "", errors.New(
				"failed to decode PEM block containing tls certificate",
			)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, "", "", err
		}
		pool := x509.NewCertPool()
		pool.AddCert(cert)

		creds = credentials.NewClientTLSFromCert(pool, "")
		tlsQueryString = "&cert=" + base64.StdEncoding.EncodeToString(tlsBytes)
		if macQueryString == "" {
			tlsQueryString = "?cert=" + base64.StdEncoding.EncodeToString(tlsBytes)
		}
	}

	conn, err = grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, "", "", err
	}

	// derive lnConnect URL
	lnConnectUrl := fmt.Sprintf("lndconnect://%s%s%s", host, macQueryString, tlsQueryString)

	return conn, macaroon, lnConnectUrl, nil
}

func deriveLndNetwork(network string) string {
	switch network {
	case "bitcoin":
		return "mainnet"
	case "testnet":
		return "testnet"
	case "testnet4":
		return "testnet"
	case "signet":
		return "signet"
	case "regtest":
		return "regtest"
	case "mutinynet":
		return "signet"
	default:
		return "regtest"
	}
}
