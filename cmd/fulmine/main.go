package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ArkLabsHQ/fulmine/internal/config"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	lnd "github.com/ArkLabsHQ/fulmine/internal/infrastructure/lnd"
	scheduler "github.com/ArkLabsHQ/fulmine/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/fulmine/internal/interface/grpc"
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

	log.Info("starting fulmine...")

	svcConfig := grpcservice.Config{
		GRPCPort: cfg.GRPCPort,
		HTTPPort: cfg.HTTPPort,
		WithTLS:  cfg.WithTLS,
	}

	storeCfg := store.Config{
		BaseDir:          cfg.Datadir,
		ConfigStoreType:  configStoreType,
		AppDataStoreType: appDataStoreType,
	}
	storeSvc, err := store.NewStore(storeCfg)
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	dbSvc, err := db.NewService(db.ServiceConfig{
		DbType:   "badger",
		DbConfig: []any{cfg.Datadir, log.New()},
	})
	if err != nil {
		log.WithError(err).Fatal("failed to open db")
	}

	buildInfo := application.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	schedulerSvc := scheduler.NewScheduler()
	lnSvc := lnd.NewService()

	appSvc, err := application.NewService(
		buildInfo, storeCfg, storeSvc, dbSvc, schedulerSvc, lnSvc, cfg.EsploraURL,
	)
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	svc, err := grpcservice.NewService(svcConfig, appSvc, cfg.UnlockerService())
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
