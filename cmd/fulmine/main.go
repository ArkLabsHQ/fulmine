package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/config"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	scheduler "github.com/ArkLabsHQ/fulmine/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/fulmine/internal/interface/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	log "github.com/sirupsen/logrus"
)

// nolint:all
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	sentryDsn = ""
)

const (
	configStoreType  = types.FileStore
	appDataStoreType = types.SQLStore
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.WithError(err).Fatal("invalid config")
	}

	log.SetLevel(log.Level(cfg.LogLevel))

	// Start pprof server
	if cfg.ProfilingEnabled {
		go func() {
			pprofAddr := ":6060"
			log.Infof("starting pprof server on %s", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.WithError(err).Error("pprof server failed")
			}
		}()
	}

	sentryEnabled := !cfg.DisableTelemetry && sentryDsn != ""

	if sentryEnabled {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              sentryDsn,
			Environment:      "prod",
			AttachStacktrace: true,
			Release:          version,
		}); err != nil {
			log.Fatal(err)
		}

		sentryLevels := []log.Level{log.ErrorLevel, log.FatalLevel, log.PanicLevel}
		sentryHook, err := sentrylogrus.New(sentryLevels, sentry.ClientOptions{
			Dsn:              sentryDsn,
			Debug:            true,
			AttachStacktrace: true,
		})
		if err != nil {
			log.Fatal(err)
		}

		log.AddHook(sentryHook)

		defer func() {
			sentry.Flush(5 * time.Second)
			sentryHook.Flush(5 * time.Second)
		}()
	}

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
		DbType:   cfg.DbType,
		DbConfig: []any{cfg.Datadir},
	})
	if err != nil {
		log.WithError(err).Fatal("failed to open db")
	}

	buildInfo := application.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	pollInterval := time.Duration(cfg.SchedulerPollInterval) * time.Second
	schedulerSvc := scheduler.NewScheduler(cfg.EsploraURL, pollInterval)

	appSvc, err := application.NewService(
		buildInfo, storeCfg, storeSvc, dbSvc, schedulerSvc,
		cfg.EsploraURL, cfg.BoltzURL, cfg.BoltzWSURL, cfg.SwapTimeout,
		cfg.LnConnectionOpts, cfg.RefreshDbInterval,
	)
	if err != nil {
		log.WithError(err).Fatal("failed to init application service")
	}

	delegatorSvc := application.NewDelegatorService(appSvc, 0)

	svc, err := grpcservice.NewService(
		svcConfig, appSvc, delegatorSvc, cfg.UnlockerService(), sentryEnabled, 
		cfg.MacaroonSvc(), cfg.ArkServer, cfg.OtelCollectorURL, cfg.OtelPushInterval, cfg.PyroscopeURL,
	)
	if err != nil {
		log.WithError(err).Fatal("failed to init interface service")
	}

	log.RegisterExitHandler(svc.Stop)

	log.Info("starting service...")
	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	delegatorSvc.Stop()

	log.Info("shutting down service...")
	log.Exit(0)
}
