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
	BoltzURL         string
	BoltzWSURL       string
	CLNDatadir       string // for testing purposes only
	UnlockerType     string
	UnlockerFilePath string
	UnlockerPassword string
	DisableTelemetry bool

	unlocker ports.Unlocker
}

var (
	Datadir          = "DATADIR"
	GRPCPort         = "GRPC_PORT"
	HTTPPort         = "HTTP_PORT"
	WithTLS          = "WITH_TLS"
	LogLevel         = "LOG_LEVEL"
	ArkServer        = "ARK_SERVER"
	EsploraURL       = "ESPLORA_URL"
	BoltzURL         = "BOLTZ_URL"
	BoltzWSURL       = "BOLTZ_WS_URL"
	DisableTelemetry = "DISABLE_TELEMETRY"

	// Only for testing purposes
	CLNDatadir = "CLN_DATADIR"

	// Unlocker configuration
	UnlockerType     = "UNLOCKER_TYPE"
	UnlockerFilePath = "UNLOCKER_FILE_PATH"
	UnlockerPassword = "UNLOCKER_PASSWORD"

	defaultDatadir          = appDatadir("fulmine", false)
	defaultGRPCPort         = 7000
	defaultHTTPPort         = 7001
	defaultWithTLS          = false
	defaultLogLevel         = 4
	defaultArkServer        = ""
	defaultDisableTelemetry = false
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
	viper.SetDefault(DisableTelemetry, defaultDisableTelemetry)

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
		BoltzURL:         viper.GetString(BoltzURL),
		BoltzWSURL:       viper.GetString(BoltzWSURL),
		CLNDatadir:       cleanAndExpandPath(viper.GetString(CLNDatadir)),
		UnlockerType:     viper.GetString(UnlockerType),
		UnlockerFilePath: viper.GetString(UnlockerFilePath),
		UnlockerPassword: viper.GetString(UnlockerPassword),
		DisableTelemetry: viper.GetBool(DisableTelemetry),
	}

	if err := config.initUnlockerService(); err != nil {
		return nil, err
	}

	return config, nil
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
