package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	grpcservice "github.com/ArkLabsHQ/ark-node/internal/interface/grpc"
	log "github.com/sirupsen/logrus"
)

//nolint:all
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// TODO: Edit this file to something more meaningful for your application.
func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.WithError(err).Fatal("invalid config")
	}

	log.SetLevel(log.Level(cfg.LogLevel))

	svcConfig := grpcservice.Config{
		Port:    cfg.Port,
		WithTLS: cfg.WithTLS,
	}

	svc, err := grpcservice.NewService(svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	log.RegisterExitHandler(svc.Stop)

	log.Info("starting service...")
	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	log.Info("shutting down service...")
	log.Exit(0)
}
