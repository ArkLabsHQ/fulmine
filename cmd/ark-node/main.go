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
	"github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	log "github.com/sirupsen/logrus"
)

// nolint:all
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	configStoreType  = types.FileStore
	appDataStoreType = types.KVStore
)

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

	storeSvc, err := store.NewStore(store.Config{
		BaseDir:          cfg.Datadir,
		ConfigStoreType:  configStoreType,
		AppDataStoreType: appDataStoreType,
	})
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
	lnSvc := lnd.NewService()

	appSvc, err := application.NewService(buildInfo, storeSvc, settingsRepo, schedulerSvc, lnSvc)
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
