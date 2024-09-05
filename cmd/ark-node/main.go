package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	grpcservice "github.com/ArkLabsHQ/ark-node/internal/interface/grpc"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	filestore "github.com/ark-network/ark/pkg/client-sdk/store/file"
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

	// Initialize the ARK SDK

	log.Info("starting ark-node...")
	log.Infof("dust: %d", arksdk.DUST)

	svcConfig := grpcservice.Config{
		Port:    cfg.Port,
		WithTLS: cfg.WithTLS,
	}

	storeSvc, err := filestore.NewConfigStore(cfg.Datadir)
	if err != nil {
		log.WithError(err).Fatal(err)
	}
	settingsRepo, err := badgerdb.NewSettingsRepo(cfg.Datadir, log.New())
	if err != nil {
		log.WithError(err).Fatal(err)
	}
	appSvc, err := application.NewService(storeSvc, settingsRepo)
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	svc, err := grpcservice.NewService(svcConfig, appSvc)
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
