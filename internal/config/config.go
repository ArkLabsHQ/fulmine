package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	envunlocker "github.com/ArkLabsHQ/fulmine/internal/infrastructure/unlocker/env"
	fileunlocker "github.com/ArkLabsHQ/fulmine/internal/infrastructure/unlocker/file"
	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/spf13/viper"
)

const (
	sqliteDb = "sqlite"
	badgerDb = "badger"
)

type Config struct {
	Datadir    string
	DbType     string
	GRPCPort   uint32
	HTTPPort   uint32
	WithTLS    bool
	LogLevel   uint32
	ArkServer  string
	EsploraURL string
	BoltzURL   string
	BoltzWSURL string

	UnlockerType     string
	UnlockerFilePath string
	UnlockerPassword string
	DisableTelemetry bool

	LnConnectionOpts *domain.LnConnectionOpts

	unlocker    ports.Unlocker
	macaroonSvc macaroon.Service
}

var (
	Datadir          = "DATADIR"
	DbType           = "DB_TYPE"
	GRPCPort         = "GRPC_PORT"
	HTTPPort         = "HTTP_PORT"
	WithTLS          = "WITH_TLS"
	LogLevel         = "LOG_LEVEL"
	ArkServer        = "ARK_SERVER"
	EsploraURL       = "ESPLORA_URL"
	BoltzURL         = "BOLTZ_URL"
	BoltzWSURL       = "BOLTZ_WS_URL"
	DisableTelemetry = "DISABLE_TELEMETRY"
	NoMacaroons      = "NO_MACAROONS"
	LndUrl           = "LND_URL"
	ClnUrl           = "CLN_URL"
	ClnDatadir       = "CLN_DATADIR"
	LndDatadir       = "LND_DATADIR"

	// Unlocker configuration
	UnlockerType     = "UNLOCKER_TYPE"
	UnlockerFilePath = "UNLOCKER_FILE_PATH"
	UnlockerPassword = "UNLOCKER_PASSWORD"

	defaultDatadir          = appDatadir("fulmine", false)
	dbType                  = sqliteDb
	defaultGRPCPort         = 7000
	defaultHTTPPort         = 7001
	defaultWithTLS          = false
	defaultLogLevel         = 4
	defaultArkServer        = ""
	defaultDisableTelemetry = false
	supportedDbType         = map[string]struct{}{
		sqliteDb: {},
		badgerDb: {},
	}
	defaultNoMacaroons = false
	defaultLndUrl      = ""
	defaultClnUrl      = ""
	defaultClnDatadir  = ""
	defaultLndDatadir  = ""
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
	viper.SetDefault(DbType, dbType)
	viper.SetDefault(NoMacaroons, defaultNoMacaroons)
	viper.SetDefault(LndUrl, defaultLndUrl)
	viper.SetDefault(ClnUrl, defaultClnUrl)
	viper.SetDefault(ClnDatadir, defaultClnDatadir)
	viper.SetDefault(LndDatadir, defaultLndDatadir)

	if err := initDatadir(); err != nil {
		return nil, fmt.Errorf("error while creating datadir: %s", err)
	}

	if _, ok := supportedDbType[viper.GetString(DbType)]; !ok {
		return nil, fmt.Errorf("unsupported db type: %s", viper.GetString(DbType))
	}

	lndUrl := viper.GetString(LndUrl)
	clnUrl := viper.GetString(ClnUrl)

	lndDatadir := cleanAndExpandPath(viper.GetString(LndDatadir))
	clnDatadir := cleanAndExpandPath(viper.GetString(ClnDatadir))

	lnConnectionOpts, err := deriveLnConfig(lndUrl, clnUrl, lndDatadir, clnDatadir)
	if err != nil {
		return nil, fmt.Errorf("error deriving lightning connection config: %w", err)
	}

	config := &Config{
		Datadir:          viper.GetString(Datadir),
		DbType:           viper.GetString(DbType),
		GRPCPort:         viper.GetUint32(GRPCPort),
		HTTPPort:         viper.GetUint32(HTTPPort),
		WithTLS:          viper.GetBool(WithTLS),
		LogLevel:         viper.GetUint32(LogLevel),
		ArkServer:        viper.GetString(ArkServer),
		EsploraURL:       viper.GetString(EsploraURL),
		BoltzURL:         viper.GetString(BoltzURL),
		BoltzWSURL:       viper.GetString(BoltzWSURL),
		UnlockerType:     viper.GetString(UnlockerType),
		UnlockerFilePath: viper.GetString(UnlockerFilePath),
		UnlockerPassword: viper.GetString(UnlockerPassword),
		DisableTelemetry: viper.GetBool(DisableTelemetry),

		LnConnectionOpts: lnConnectionOpts,
	}

	if err := config.initUnlockerService(); err != nil {
		return nil, err
	}

	if err := config.initMacaroonService(); err != nil {
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

func (c Config) MacaroonSvc() macaroon.Service {
	return c.macaroonSvc
}

func (c *Config) initMacaroonService() error {
	if c.macaroonSvc != nil {
		return nil
	}

	if !viper.GetBool(NoMacaroons) {
		svc, err := macaroon.NewService(
			viper.GetString(Datadir), macFiles, WhitelistedByMethod(), AllPermissionsByMethod(),
		)
		if err != nil {
			return err
		}

		c.macaroonSvc = svc
	}

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

func deriveLnConfig(lndUrl, clnUrl, lndDatadir, clnDatadir string) (*domain.LnConnectionOpts, error) {
	if lndUrl == "" && clnUrl == "" {
		return nil, nil
	}

	if lndUrl != "" && clnUrl != "" {
		return nil, fmt.Errorf("cannot set both LND and CLN URLs at the same time")
	}

	if lndDatadir != "" && clnDatadir != "" {
		return nil, fmt.Errorf("cannot set both LND and CLN datadirs at the same time")
	}

	if lndUrl != "" {
		if strings.HasPrefix(lndUrl, "lndconnect://") {
			return &domain.LnConnectionOpts{
				LnUrl:          lndUrl,
				ConnectionType: domain.LND_CONNECTION,
			}, nil
		}

		if lndDatadir == "" {
			return nil, fmt.Errorf("LND URL provided without LND datadir")
		}

		if _, err := utils.ValidateURL(lndUrl); err != nil {
			return nil, fmt.Errorf("invalid LND URL: %v", err)
		}
		return &domain.LnConnectionOpts{
			LnUrl:          lndUrl,
			LnDatadir:      lndDatadir,
			ConnectionType: domain.LND_CONNECTION,
		}, nil
	}

	if strings.HasPrefix(clnUrl, "clnconnect://") {
		return &domain.LnConnectionOpts{
			LnUrl:          clnUrl,
			ConnectionType: domain.CLN_CONNECTION,
		}, nil
	}

	if clnDatadir == "" {
		return nil, fmt.Errorf("CLN URL provided without CLN datadir")
	}

	if _, err := utils.ValidateURL(clnUrl); err != nil {
		return nil, fmt.Errorf("invalid CLN URL: %v", err)
	}

	return &domain.LnConnectionOpts{
		LnUrl:          clnUrl,
		LnDatadir:      clnDatadir,
		ConnectionType: domain.CLN_CONNECTION,
	}, nil
}
