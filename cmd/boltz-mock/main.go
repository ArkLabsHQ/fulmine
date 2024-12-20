package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	URL  = "localhost:7000"
	port = 9000
)

func main() {
	logrus.SetLevel(logrus.Level(logrus.DebugLevel))
	handler := newBoltzMockHandler(URL)
	creds := insecure.NewCredentials()

	grpcServer := grpc.NewServer(grpc.Creds(creds))

	pb.RegisterServiceServer(grpcServer, handler)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logrus.Fatalf("failed to listen: %v", err)
		return
	}

	logrus.Infof("listening on :%d", port)
	if err := grpcServer.Serve(ln); err != nil {
		logrus.Fatalf("failed to serve: %v", err)
		logrus.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	logrus.Info("shutting down service...")
	logrus.Exit(0)
}
