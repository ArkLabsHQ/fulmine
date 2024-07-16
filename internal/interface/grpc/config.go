package grpc_interface

import (
	"crypto/tls"
	"fmt"
	"net"
)

type Config struct {
	Port    uint32
	WithTLS bool
}

func (c Config) Validate() error {
	lis, err := net.Listen("tcp", c.address())
	if err != nil {
		return fmt.Errorf("invalid port: %s", err)
	}
	defer lis.Close()

	if c.WithTLS {
		return fmt.Errorf("tls termination not supported yet")
	}
	return nil
}

func (c Config) insecure() bool {
	return !c.WithTLS
}

func (c Config) address() string {
	return fmt.Sprintf(":%d", c.Port)
}

func (c Config) gatewayAddress() string {
	return fmt.Sprintf("localhost:%d", c.Port)
}

func (c Config) tlsConfig() *tls.Config {
	return nil
}
