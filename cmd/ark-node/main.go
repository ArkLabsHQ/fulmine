package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	lnd "github.com/ArkLabsHQ/ark-node/internal/infrastructure/lnd"
	scheduler "github.com/ArkLabsHQ/ark-node/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/ark-node/internal/interface/grpc"
	filestore "github.com/ark-network/ark/pkg/client-sdk/store/file"
	log "github.com/sirupsen/logrus"
)

// nolint:all
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

	svcConfig := grpcservice.Config{
		GRPCPort: cfg.GRPCPort,
		HTTPPort: cfg.HTTPPort,
		WithTLS:  cfg.WithTLS,
	}

	storeSvc, err := filestore.NewConfigStore(cfg.Datadir)
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	settingsRepo, err := badgerdb.NewSettingsRepo(cfg.Datadir, log.New())
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	buildInfo := application.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	schedulerSvc := scheduler.NewScheduler()
	lndSvc := lnd.NewService()

	appSvc, err := application.NewService(buildInfo, storeSvc, settingsRepo, schedulerSvc, lndSvc)
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
