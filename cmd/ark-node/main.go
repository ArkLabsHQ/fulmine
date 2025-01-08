package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	cln_grpc "github.com/ArkLabsHQ/ark-node/internal/infrastructure/cln-grpc"
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

	vhtlcRepo, err := badgerdb.NewVHTLCRepo(cfg.Datadir, log.New())
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
	if cfg.CLNDatadir != "" {
		rootCert := filepath.Join(cfg.CLNDatadir, "ca.pem")
		privateKey := filepath.Join(cfg.CLNDatadir, "client-key.pem")
		certChain := filepath.Join(cfg.CLNDatadir, "client.pem")
		lnSvc, err = cln_grpc.NewService(rootCert, privateKey, certChain)
		if err != nil {
			log.WithError(err).Fatal("failed to init cln client")
		}
	}

	appSvc, err := application.NewService(
		buildInfo,
		storeSvc,
		settingsRepo,
		vhtlcRepo,
		schedulerSvc,
		lnSvc,
	)
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
