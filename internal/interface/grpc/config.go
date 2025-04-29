package grpc_interface

import (
	"crypto/tls"
	"fmt"
	"net"
)

type Config struct {
	GRPCPort uint32
	HTTPPort uint32
	WithTLS  bool
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

func (c Config) gatewayAddress() string {
	return fmt.Sprintf("localhost:%d", c.GRPCPort)
}

func (c Config) tlsConfig() *tls.Config {
	return nil
}
