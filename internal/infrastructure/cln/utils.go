package cln

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

	if rootCert != "" {
		rootCert = "-----BEGIN CERTIFICATE-----\n" + rootCert + "\n-----END CERTIFICATE-----"
	}
	if certChain != "" {
		certChain = "-----BEGIN CERTIFICATE-----\n" + certChain + "\n-----END CERTIFICATE-----"
	}
	if privateKey != "" {
		privateKey = "-----BEGIN PRIVATE KEY-----\n" + privateKey + "\n-----END PRIVATE KEY-----"
	}

	return

}

func deriveClnConnFromUrl(clnConnectUrl string) (conn *grpc.ClientConn, err error) {
	rootCert, privateKey, certChain, host, err := decodeClnConnectUrl(clnConnectUrl)
	if err != nil {
		return nil, fmt.Errorf("error decoding cln connect url: %w", err)
	}

	creds := insecure.NewCredentials()
	if rootCert != "" {
		creds, err = deriveCreds([]byte(rootCert), []byte(certChain), []byte(privateKey))
		if err != nil {
			return nil, fmt.Errorf("error deriving credentials: %w", err)
		}
	}

	return grpc.NewClient(host, grpc.WithTransportCredentials(creds))

}

func deriveClnConnFromPath(dataDir, host, network string) (conn *grpc.ClientConn, lnConnectUrl string, err error) {
	dataDir = filepath.Join(dataDir, network)
	rootCertPath := filepath.Join(dataDir, "ca.pem")           // root certificate
	certChainPath := filepath.Join(dataDir, "client.pem")      // client certificate chain
	privateKeyPath := filepath.Join(dataDir, "client-key.pem") // client private key
	creds := insecure.NewCredentials()
	tlsQueryString := ""
	if _, err := os.Stat(rootCertPath); err == nil {
		rootCertBytes, err := os.ReadFile(rootCertPath)
		if err != nil {
			return nil, "", err
		}
		certChainBytes, err := os.ReadFile(certChainPath)
		if err != nil {
			return nil, "", err
		}
		privateKeyBytes, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return nil, "", err
		}

		creds, err = deriveCreds(rootCertBytes, certChainBytes, privateKeyBytes)
		if err != nil {
			return nil, "", fmt.Errorf("error deriving credentials: %w", err)
		}
		tlsQueryString = fmt.Sprintf(
			"?rootCert=%s&privateKey=%s&certChain=%s",
			url.QueryEscape(toBase64(string(rootCertBytes))),
			url.QueryEscape(toBase64(string(privateKeyBytes))),
			url.QueryEscape(toBase64(string(certChainBytes))))
	}

	//derive LnConnect URL
	lnConnectUrl = fmt.Sprintf("clnconnect://%s%s", host, tlsQueryString)
	conn, err = grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, "", fmt.Errorf("error creating grpc client: %w", err)
	}

	return conn, lnConnectUrl, nil

}

func deriveCreds(rootCert []byte, certChain []byte, privateKey []byte) (credentials.TransportCredentials, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(rootCert) {
		return nil, fmt.Errorf("could not parse root certificate")
	}

	cert, err := tls.X509KeyPair(certChain, privateKey)
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
