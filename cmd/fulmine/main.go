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
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/pricefeed"
	scheduler "github.com/ArkLabsHQ/fulmine/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/fulmine/internal/interface/grpc"
	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// nolint:all
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	sentryDsn = ""
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
		GRPCPort:      cfg.GRPCPort,
		HTTPPort:      cfg.HTTPPort,
		DelegatorPort: cfg.DelegatorPort,
		WithTLS:       cfg.WithTLS,
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

	var introClient introclient.TransportClient
	if cfg.TakerEnabled && cfg.IntrospectorURL != "" {
		introConn, err := grpc.NewClient(cfg.IntrospectorURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.WithError(err).Fatal("failed to connect to introspector")
		}
		introClient = introclient.NewGRPCClient(introConn)
	}

	takerConfig := application.TakerConfig{
		Enabled:     cfg.TakerEnabled,
		IntroClient: introClient,
		PriceFeed:   pricefeed.NewCoinGeckoPriceFeed(),
	}

	appSvc, delegatorSvc, takerSvc, err := application.NewServices(
		buildInfo, cfg.Datadir, dbSvc, schedulerSvc,
		cfg.EsploraURL, cfg.BoltzURL, cfg.BoltzWSURL, cfg.SwapTimeout,
		cfg.LnConnectionOpts, cfg.RefreshDbInterval,
		application.DelegatorConfig{
			Enabled: cfg.DelegatorEnabled,
			Fee:     cfg.DelegatorFee,
		},
		takerConfig,
	)
	if err != nil {
		log.WithError(err).Fatal("failed to init application service")
	}

	svc, err := grpcservice.NewService(
		svcConfig, appSvc, delegatorSvc, takerSvc, cfg.UnlockerService(), sentryEnabled,
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

	log.Info("shutting down service...")
	log.Exit(0)
}
