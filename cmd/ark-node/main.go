package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	scheduler "github.com/ArkLabsHQ/ark-node/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/ark-node/internal/interface/grpc"
	filestore "github.com/ark-network/ark/pkg/client-sdk/store/file"
	log "github.com/sirupsen/logrus"
	"github.com/skratchdot/open-golang/open"
)

// nolint:all
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.WithError(err).Fatal("invalid config")
	}

	log.SetLevel(log.Level(cfg.LogLevel))

	log.Info("starting ark-node...")

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

	buildInfo := application.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	schedulerSvc := scheduler.NewScheduler()

	appSvc, err := application.NewService(buildInfo, storeSvc, settingsRepo, schedulerSvc)
	if err != nil {
		log.WithError(err).Fatal(err)
	}

	svc, err := grpcservice.NewService(svcConfig, appSvc)
	if err != nil {
		log.Fatal(err)
	}

	log.RegisterExitHandler(svc.Stop)

	// Start the gRPC service
	go func() {
		log.Info("starting service...")
		if err := svc.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait a bit for the server to start
	time.Sleep(2 * time.Second)

	// Open the browser
	url := fmt.Sprintf("http://localhost:%d", cfg.Port)
	log.Infof("Opening browser at %s", url)
	go openBrowser(url)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	log.Info("shutting down service...")
	svc.Stop()
	log.Exit(0)
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = open.Run(url)
	}

	if err != nil {
		log.WithError(err).Error("Error opening browser")
	}
}
