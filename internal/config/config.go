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
	"github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"github.com/spf13/viper"
)

const (
	sqliteDb = "sqlite"
	badgerDb = "badger"
)

//go:generate go run ../../tools/gen-env-doc/main.go
type Config struct {
	Datadir               string `mapstructure:"DATADIR" envInfo:"Data directory for Fulmine state (defaults to an OS-specific app data dir)"`
	DbType                string `mapstructure:"DB_TYPE" envDefault:"sqlite" envInfo:"Database backend: sqlite or badger"`
	GRPCPort              uint32 `mapstructure:"GRPC_PORT" envDefault:"7000" envInfo:"gRPC server port"`
	HTTPPort              uint32 `mapstructure:"HTTP_PORT" envDefault:"7001" envInfo:"HTTP server port"`
	WithTLS               bool   `mapstructure:"WITH_TLS" envDefault:"false" envInfo:"Enable TLS on the server"`
	LogLevel              uint32 `mapstructure:"LOG_LEVEL" envDefault:"4" envInfo:"Log verbosity (higher = more verbose)"`
	ArkServer             string `mapstructure:"ARK_SERVER" envInfo:"Ark server address (e.g., arkd:7070)"`
	EsploraURL            string `mapstructure:"ESPLORA_URL" envInfo:"Esplora base URL (e.g., http://chopsticks:3000)"`
	BoltzURL              string `mapstructure:"BOLTZ_URL" envInfo:"Boltz HTTP endpoint (e.g., http://boltz:9001)"`
	BoltzWSURL            string `mapstructure:"BOLTZ_WS_URL" envInfo:"Boltz WebSocket endpoint (e.g., ws://boltz:9002)"`
	LnurlServerURL        string `mapstructure:"LNURL_SERVER_URL" envInfo:"lnurl-server base URL for amountless Lightning receive (e.g. http://lnurl-server:3000); empty disables it"`
	SchedulerPollInterval int64  `mapstructure:"SCHEDULER_POLL_INTERVAL" envDefault:"600" envInfo:"Scheduler polling interval in seconds"`
	ProfilingEnabled      bool   `mapstructure:"PROFILING_ENABLED" envDefault:"false" envInfo:"Enable profiling endpoints"`
	RefreshDbInterval     int64  `mapstructure:"REFRESH_DB_INTERVAL" envDefault:"60" envInfo:"Interval in seconds to refresh the database with latest blockchain data"`
	DelegatePort          uint32 `mapstructure:"DELEGATE_PORT" envDefault:"7002" envInfo:"Delegate server port"`
	DelegateFee           uint64 `mapstructure:"DELEGATE_FEE" envDefault:"0" envInfo:"Fee the delegate charges, in satoshis"`
	DelegateEnabled       bool   `mapstructure:"DELEGATE_ENABLED" envDefault:"false" envInfo:"Run the delegate server"`

	UnlockerType     string `mapstructure:"UNLOCKER_TYPE" envInfo:"Unlocker type: file or env"`
	UnlockerFilePath string `mapstructure:"UNLOCKER_FILE_PATH" envInfo:"Path to the unlocker password file (file unlocker)"`
	UnlockerPassword string `mapstructure:"UNLOCKER_PASSWORD" envInfo:"Unlocker password (env unlocker)"`
	DisableTelemetry bool   `mapstructure:"DISABLE_TELEMETRY" envDefault:"false" envInfo:"Disable telemetry"`
	SwapTimeout      uint32 `mapstructure:"SWAP_TIMEOUT" envDefault:"15" envInfo:"Swap timeout in seconds"`
	OtelCollectorURL string `mapstructure:"OTEL_COLLECTOR_URL" envInfo:"OpenTelemetry collector URL; enables OTel export when set"`
	OtelPushInterval int64  `mapstructure:"OTEL_PUSH_INTERVAL" envDefault:"10" envInfo:"OpenTelemetry metrics push interval in seconds"`
	PyroscopeURL     string `mapstructure:"PYROSCOPE_URL" envInfo:"Pyroscope server URL for continuous profiling when set"`

	unlocker    ports.Unlocker
	macaroonSvc macaroon.Service
}

var (
	Datadir               = "DATADIR"
	DbType                = "DB_TYPE"
	GRPCPort              = "GRPC_PORT"
	HTTPPort              = "HTTP_PORT"
	WithTLS               = "WITH_TLS"
	LogLevel              = "LOG_LEVEL"
	ArkServer             = "ARK_SERVER"
	EsploraURL            = "ESPLORA_URL"
	BoltzURL              = "BOLTZ_URL"
	BoltzWSURL            = "BOLTZ_WS_URL"
	DisableTelemetry      = "DISABLE_TELEMETRY"
	NoMacaroons           = "NO_MACAROONS"
	OtelCollectorURL      = "OTEL_COLLECTOR_URL"
	OtelPushInterval      = "OTEL_PUSH_INTERVAL"
	PyroscopeURL          = "PYROSCOPE_URL"
	SwapTimeout           = "SWAP_TIMEOUT"
	SchedulerPollInterval = "SCHEDULER_POLL_INTERVAL"
	ProfilingEnabled      = "PROFILING_ENABLED"
	RefreshDbInterval     = "REFRESH_DB_INTERVAL"
	DelegatePort          = "DELEGATE_PORT"
	DelegateFee           = "DELEGATE_FEE"
	DelegateEnabled       = "DELEGATE_ENABLED"

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
	defaultNoMacaroons           = false
	defaultSwapTimeout           = 15  // In seconds
	defaultSchedulerPollInterval = 600 // 10 minutes
	defaultProfilingEnabled      = false
	defaultRefreshDbInterval     = 60
	defaultOtelPushInterval      = 10 // 10 seconds
	defaultDelegatePort          = 7002
	defaultDelegateFee           = 0
	defaultDelegateEnabled       = false
)

func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("FULMINE")
	viper.AutomaticEnv()

	viper.SetDefault(Datadir, defaultDatadir)
	viper.SetDefault(GRPCPort, defaultGRPCPort)
	viper.SetDefault(HTTPPort, defaultHTTPPort)
	viper.SetDefault(DelegatePort, defaultDelegatePort)
	viper.SetDefault(WithTLS, defaultWithTLS)
	viper.SetDefault(LogLevel, defaultLogLevel)
	viper.SetDefault(ArkServer, defaultArkServer)
	viper.SetDefault(DisableTelemetry, defaultDisableTelemetry)
	viper.SetDefault(DbType, dbType)
	viper.SetDefault(NoMacaroons, defaultNoMacaroons)
	viper.SetDefault(SwapTimeout, defaultSwapTimeout)
	viper.SetDefault(SchedulerPollInterval, defaultSchedulerPollInterval)
	viper.SetDefault(ProfilingEnabled, defaultProfilingEnabled)
	viper.SetDefault(RefreshDbInterval, defaultRefreshDbInterval)
	viper.SetDefault(OtelPushInterval, defaultOtelPushInterval)
	viper.SetDefault(DelegateFee, defaultDelegateFee)
	viper.SetDefault(DelegateEnabled, defaultDelegateEnabled)

	// TODO: move to validate method
	if err := initDatadir(); err != nil {
		return nil, fmt.Errorf("error while creating datadir: %s", err)
	}

	if _, ok := supportedDbType[viper.GetString(DbType)]; !ok {
		return nil, fmt.Errorf("unsupported db type: %s", viper.GetString(DbType))
	}

	if viper.GetInt64(SchedulerPollInterval) < 1 {
		return nil, fmt.Errorf("scheduler poll interval must be at least 1 second")
	}
	if viper.GetBool(DelegateEnabled) {
		if viper.GetUint32(DelegatePort) == viper.GetUint32(GRPCPort) ||
			viper.GetUint32(DelegatePort) == viper.GetUint32(HTTPPort) {
			return nil, fmt.Errorf("delegate must not run on same port of the wallet")
		}
	}

	config := &Config{
		Datadir:               viper.GetString(Datadir),
		DbType:                viper.GetString(DbType),
		GRPCPort:              viper.GetUint32(GRPCPort),
		HTTPPort:              viper.GetUint32(HTTPPort),
		WithTLS:               viper.GetBool(WithTLS),
		LogLevel:              viper.GetUint32(LogLevel),
		ArkServer:             viper.GetString(ArkServer),
		EsploraURL:            viper.GetString(EsploraURL),
		BoltzURL:              viper.GetString(BoltzURL),
		BoltzWSURL:            viper.GetString(BoltzWSURL),
		UnlockerType:          viper.GetString(UnlockerType),
		UnlockerFilePath:      viper.GetString(UnlockerFilePath),
		UnlockerPassword:      viper.GetString(UnlockerPassword),
		DisableTelemetry:      viper.GetBool(DisableTelemetry),
		SwapTimeout:           viper.GetUint32(SwapTimeout),
		SchedulerPollInterval: viper.GetInt64(SchedulerPollInterval),
		ProfilingEnabled:      viper.GetBool(ProfilingEnabled),
		RefreshDbInterval:     viper.GetInt64(RefreshDbInterval),
		OtelCollectorURL:      viper.GetString(OtelCollectorURL),
		OtelPushInterval:      viper.GetInt64(OtelPushInterval),
		PyroscopeURL:          viper.GetString(PyroscopeURL),
		DelegatePort:          viper.GetUint32(DelegatePort),
		DelegateFee:           viper.GetUint64(DelegateFee),
		DelegateEnabled:       viper.GetBool(DelegateEnabled),
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
			c.Datadir, macaroonsFolder, macFiles, WhitelistedByMethod(), AllPermissionsByMethod(),
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
