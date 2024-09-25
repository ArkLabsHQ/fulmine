package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/ArkLabsHQ/ark-node/internal/config"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	scheduler "github.com/ArkLabsHQ/ark-node/internal/infrastructure/scheduler/gocron"
	grpcservice "github.com/ArkLabsHQ/ark-node/internal/interface/grpc"
	filestore "github.com/ark-network/ark/pkg/client-sdk/store/file"
	log "github.com/sirupsen/logrus"

	"github.com/getlantern/systray"
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
	log.Info("starting service...")
	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}

	// Attach System Tray
	systray.Run(onReady, onExit)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	log.Info("shutting down service...")
	svc.Stop()
	log.Info("service stopped")
}

func onReady() {
	systray.SetTitle("Ark Node")
	systray.SetTooltip("Ark Node is your gateway to the Ark protocol")

	mOpen := systray.AddMenuItem("Dashboard", "Open the web app in browser")
	mQuit := systray.AddMenuItem("Quit", "Quit the app")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(fmt.Sprintf("http://localhost:%d/app", 7000))
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	// Open the browser automatically when the app starts in a desktop environment
	openBrowser(fmt.Sprintf("http://localhost:%d/app", 7000))
}

func onExit() {
	// Cleanup code here
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
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		log.Println("Error opening browser:", err)
	}
}
