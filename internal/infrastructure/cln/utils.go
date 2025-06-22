package cln

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
)

func decodeClnConnectUrl(clnConnectUrl string) (rootCert, privateKey, certChain, host string, err error) {
	u, err := url.Parse(clnConnectUrl)
	if err != nil {
		return
	}

	host = u.Host

	rootCert = toBase64(u.Query().Get("rootCert"))     // ca.pem
	certChain = toBase64(u.Query().Get("certChain"))   // client.pem
	privateKey = toBase64(u.Query().Get("privateKey")) // client-key.pem

	rootCert = "-----BEGIN CERTIFICATE-----\n" + rootCert + "\n-----END CERTIFICATE-----"
	certChain = "-----BEGIN CERTIFICATE-----\n" + certChain + "\n-----END CERTIFICATE-----"
	privateKey = "-----BEGIN PRIVATE KEY-----\n" + privateKey + "\n-----END PRIVATE KEY-----"

	return
}

func parseClnPath(rootCertPath, certChainPath, privateKeyPath string) (cred credentials.TransportCredentials, err error) {
	rootCertBytes, err := os.ReadFile(rootCertPath)
	if err != nil {
		return nil, err
	}
	certChainBytes, err := os.ReadFile(certChainPath)
	if err != nil {
		return nil, err
	}
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(rootCertBytes) {
		return nil, fmt.Errorf("could not parse root certificate")
	}

	cert, err := tls.X509KeyPair(certChainBytes, privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("error with X509KeyPair, %s", err)
	}

	creds := credentials.NewTLS(&tls.Config{
		ServerName:   "cln",
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})

	return creds, nil
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
