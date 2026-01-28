package grpc_interface

import (
	"crypto/tls"
	"fmt"
	"net"
)

type Config struct {
	GRPCPort          uint32
	HTTPPort          uint32
	DelegatorGRPCPort uint32 // 0 means use the same port as GRPCPort
	DelegatorHTTPPort uint32 // 0 means use the same port as HTTPPort
	WithTLS           bool
}

func (c Config) Validate() error {
	lis, err := net.Listen("tcp", c.grpcAddress())
	if err != nil {
		return fmt.Errorf("invalid grpc port: %s", err)
	}
	// nolint:all
	lis.Close()

	lis, err = net.Listen("tcp", c.httpAddress())
	if err != nil {
		return fmt.Errorf("invalid http port: %s", err)
	}
	// nolint:all
	lis.Close()

	if c.DelegatorGRPCPort > 0 {
		lis, err = net.Listen("tcp", c.delegatorGRPCAddress())
		if err != nil {
			return fmt.Errorf("invalid delegator grpc port: %s", err)
		}
		// nolint:all
		lis.Close()
	}

	if c.DelegatorHTTPPort > 0 {
		lis, err = net.Listen("tcp", c.delegatorHTTPAddress())
		if err != nil {
			return fmt.Errorf("invalid delegator http port: %s", err)
		}
		// nolint:all
		lis.Close()
	}

	if c.WithTLS {
		return fmt.Errorf("tls termination not supported yet")
	}
	return nil
}

func (c Config) insecure() bool {
	return !c.WithTLS
}

func (c Config) grpcAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

func (c Config) httpAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

func (c Config) delegatorHTTPAddress() string {
	port := c.DelegatorHTTPPort
	if port == 0 {
		port = c.HTTPPort
	}
	return fmt.Sprintf(":%d", port)
}

func (c Config) gatewayAddress() string {
	return fmt.Sprintf("127.0.0.1:%d", c.GRPCPort)
}

func (c Config) delegatorGatewayAddress() string {
	port := c.DelegatorGRPCPort
	if port == 0 {
		port = c.GRPCPort
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func (c Config) delegatorGRPCAddress() string {
	port := c.DelegatorGRPCPort
	if port == 0 {
		port = c.GRPCPort
	}
	return fmt.Sprintf(":%d", port)
}

// hasSeparateDelegatorPort returns true if delegator should run on a separate port
func (c Config) hasSeparateDelegatorPort() bool {
	return c.DelegatorGRPCPort > 0 && c.DelegatorGRPCPort != c.GRPCPort
}

// hasSeparateDelegatorHTTPPort returns true if delegator should have a separate HTTP gateway port
func (c Config) hasSeparateDelegatorHTTPPort() bool {
	return c.DelegatorHTTPPort > 0 && c.DelegatorHTTPPort != c.HTTPPort
}

func (c Config) tlsConfig() *tls.Config {
	return nil
}
