package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	URL  = "localhost:7008"
	port = 9000
)

func main() {
	log.SetLevel(log.Level(log.DebugLevel))
	handler := newBoltzMockHandler(URL)
	creds := insecure.NewCredentials()

	grpcServer := grpc.NewServer(grpc.Creds(creds))

	pb.RegisterServiceServer(grpcServer, handler)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		return
	}

	log.Infof("listening on :%d", port)
	if err := grpcServer.Serve(ln); err != nil {
		log.Fatalf("failed to serve: %v", err)
		log.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	log.Info("shutting down service...")
	log.Exit(0)
}
