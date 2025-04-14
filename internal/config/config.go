package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	envunlocker "github.com/ArkLabsHQ/fulmine/internal/infrastructure/unlocker/env"
	fileunlocker "github.com/ArkLabsHQ/fulmine/internal/infrastructure/unlocker/file"
	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	Datadir          string
	GRPCPort         uint32
	HTTPPort         uint32
	WithTLS          bool
	LogLevel         uint32
	ArkServer        string
	EsploraURL       string
	CLNDatadir       string // for testing purposes only
	UnlockerType     string
	UnlockerFilePath string
	UnlockerPassword string
	SentryDSN        string
	SentryEnv        string

	unlocker ports.Unlocker
}

var (
	Datadir    = "DATADIR"
	GRPCPort   = "GRPC_PORT"
	HTTPPort   = "HTTP_PORT"
	WithTLS    = "WITH_TLS"
	LogLevel   = "LOG_LEVEL"
	ArkServer  = "ARK_SERVER"
	EsploraURL = "ESPLORA_URL"
	SentryDSN  = "SENTRY_DSN"
	SentryEnv  = "SENTRY_ENVIRONMENT"

	// Only for testing purposes
	CLNDatadir = "CLN_DATADIR"

	// Unlocker configuration
	UnlockerType     = "UNLOCKER_TYPE"
	UnlockerFilePath = "UNLOCKER_FILE_PATH"
	UnlockerPassword = "UNLOCKER_PASSWORD"

	defaultDatadir   = appDatadir("fulmine", false)
	defaultGRPCPort  = 7000
	defaultHTTPPort  = 7001
	defaultWithTLS   = false
	defaultLogLevel  = 4
	defaultArkServer = ""
	defaultSentryDSN = ""
	defaultSentryEnv = "development"
)

func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("FULMINE")
	viper.AutomaticEnv()

	viper.SetDefault(Datadir, defaultDatadir)
	viper.SetDefault(GRPCPort, defaultGRPCPort)
	viper.SetDefault(HTTPPort, defaultHTTPPort)
	viper.SetDefault(WithTLS, defaultWithTLS)
	viper.SetDefault(LogLevel, defaultLogLevel)
	viper.SetDefault(ArkServer, defaultArkServer)
	viper.SetDefault(SentryDSN, defaultSentryDSN)
	viper.SetDefault(SentryEnv, defaultSentryEnv)

	if err := initDatadir(); err != nil {
		return nil, fmt.Errorf("error while creating datadir: %s", err)
	}

	config := &Config{
		Datadir:          viper.GetString(Datadir),
		GRPCPort:         viper.GetUint32(GRPCPort),
		HTTPPort:         viper.GetUint32(HTTPPort),
		WithTLS:          viper.GetBool(WithTLS),
		LogLevel:         viper.GetUint32(LogLevel),
		ArkServer:        viper.GetString(ArkServer),
		EsploraURL:       viper.GetString(EsploraURL),
		CLNDatadir:       cleanAndExpandPath(viper.GetString(CLNDatadir)),
		SentryDSN:        viper.GetString(SentryDSN),
		SentryEnv:        viper.GetString(SentryEnv),
		UnlockerType:     viper.GetString(UnlockerType),
		UnlockerFilePath: viper.GetString(UnlockerFilePath),
		UnlockerPassword: viper.GetString(UnlockerPassword),
	}

	if config.SentryDSN != "" {
		if err := initSentry(config); err != nil {
			return nil, fmt.Errorf("error initializing Sentry: %s", err)
		}
	}

	if err := config.initUnlockerService(); err != nil {
		return nil, err
	}

	return config, nil
}

func initSentry(config *Config) error {
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              config.SentryDSN,
		Environment:      config.SentryEnv,
		AttachStacktrace: true,
		ServerName:       GetHostname(),
	})
	if err != nil {
		return err
	}

	setupLogrusHook()

	return nil
}

func setupLogrusHook() {
	levels := []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
	}

	log.AddHook(&sentryHook{
		levels: levels,
	})
}

type sentryHook struct {
	levels []log.Level
}

func (h *sentryHook) Levels() []log.Level {
	return h.levels
}

func (h *sentryHook) Fire(entry *log.Entry) error {
	if sentry.CurrentHub().Client() == nil {
		return nil
	}

	// Skip if explicitly marked to skip Sentry
	if skip, ok := entry.Data["skip_sentry"].(bool); ok && skip {
		return nil
	}

	event := sentry.NewEvent()
	event.Level = getSentryLevel(entry.Level)
	event.Message = entry.Message
	event.Extra = make(map[string]interface{})

	for k, v := range entry.Data {
		if k == "error" || k == "skip_sentry" {
			continue
		}
		event.Extra[k] = v
	}

	if err, ok := entry.Data["error"].(error); ok {
		event.Exception = []sentry.Exception{{
			Value:      err.Error(),
			Type:       fmt.Sprintf("%T", err),
			Stacktrace: sentry.ExtractStacktrace(err),
		}}
	}

	sentry.CaptureEvent(event)
	return nil
}

func getSentryLevel(level log.Level) sentry.Level {
	switch level {
	case log.PanicLevel, log.FatalLevel:
		return sentry.LevelFatal
	case log.ErrorLevel:
		return sentry.LevelError
	case log.WarnLevel:
		return sentry.LevelWarning
	case log.InfoLevel:
		return sentry.LevelInfo
	case log.DebugLevel, log.TraceLevel:
		return sentry.LevelDebug
	default:
		return sentry.LevelError
	}
}

func (c *Config) IsSentryEnabled() bool {
	return c.SentryDSN != ""
}

func (c *Config) UnlockerService() ports.Unlocker {
	return c.unlocker
}

func (c *Config) initUnlockerService() error {
	if len(c.UnlockerType) <= 0 {
		return nil
	}

	var svc ports.Unlocker
	var err error
	switch c.UnlockerType {
	case "file":
		svc, err = fileunlocker.NewService(c.UnlockerFilePath)
	case "env":
		svc, err = envunlocker.NewService(c.UnlockerPassword)
	default:
		err = fmt.Errorf("unknown unlocker type")
	}
	if err != nil {
		return err
	}
	c.unlocker = svc
	return nil
}

func initDatadir() error {
	datadir := viper.GetString(Datadir)
	return makeDirectoryIfNotExists(datadir)
}

func makeDirectoryIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModeDir|0755)
	}
	return nil
}

// appDataDir returns an operating system specific directory to be used for
// storing application data for an application.  See AppDataDir for more
// details.  This unexported version takes an operating system argument
// primarily to enable the testing package to properly test the function by
// forcing an operating system that is not the currently one.
func appDatadir(appName string, roaming bool) string {
	if appName == "" || appName == "." {
		return "."
	}

	// The caller really shouldn't prepend the appName with a period, but
	// if they do, handle it gracefully by trimming it.
	appName = strings.TrimPrefix(appName, ".")
	appNameUpper := string(unicode.ToUpper(rune(appName[0]))) + appName[1:]
	appNameLower := string(unicode.ToLower(rune(appName[0]))) + appName[1:]

	// Get the OS specific home directory via the Go standard lib.
	var homeDir string
	usr, err := user.Current()
	if err == nil {
		homeDir = usr.HomeDir
	}

	// Fall back to standard HOME environment variable that works
	// for most POSIX OSes if the directory from the Go standard
	// lib failed.
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}

	goos := runtime.GOOS
	switch goos {
	// Attempt to use the LOCALAPPDATA or APPDATA environment variable on
	// Windows.
	case "windows":
		// Windows XP and before didn't have a LOCALAPPDATA, so fallback
		// to regular APPDATA when LOCALAPPDATA is not set.
		appData := os.Getenv("LOCALAPPDATA")
		if roaming || appData == "" {
			appData = os.Getenv("APPDATA")
		}

		if appData != "" {
			return filepath.Join(appData, appNameUpper)
		}

	case "darwin":
		if homeDir != "" {
			return filepath.Join(homeDir, "Library",
				"Application Support", appNameUpper)
		}

	case "plan9":
		if homeDir != "" {
			return filepath.Join(homeDir, appNameLower)
		}

	default:
		if homeDir != "" {
			return filepath.Join(homeDir, "."+appNameLower)
		}
	}

	// Fall back to the current directory if all else fails.
	return "."
}

func cleanAndExpandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		var homeDir string
		u, err := user.Current()
		if err == nil {
			homeDir = u.HomeDir
		} else {
			homeDir = os.Getenv("HOME")
		}

		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but the variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// getHostname returns the hostname of the current machine
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
