package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
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
	Datadir    string `mapstructure:"DATADIR" envDefault:"fulmine" envInfo:"Data directory for Fulmine state"`
	DbType     string `mapstructure:"DB_TYPE" envDefault:"sqlite" envInfo:"Database backend: sqlite | badger"`
	GRPCPort   uint32 `mapstructure:"GRPC_PORT" envDefault:"7000" envInfo:"gRPC server port"`
	HTTPPort   uint32 `mapstructure:"HTTP_PORT" envDefault:"7001" envInfo:"HTTP server port"`
	WithTLS    bool   `mapstructure:"WITH_TLS" envDefault:"false" envInfo:"Enable TLS on server"`
	LogLevel   uint32 `mapstructure:"LOG_LEVEL" envDefault:"4" envInfo:"Log verbosity (higher = more verbose)"`
	ArkServer  string `mapstructure:"ARK_SERVER" envDefault:"" envInfo:"Ark server address (e.g., arkd:7070)"`
	EsploraURL string `mapstructure:"ESPLORA_URL" envDefault:"" envInfo:"Esplora base URL (e.g., http://chopsticks:3000)"`
	BoltzURL   string `mapstructure:"BOLTZ_URL" envDefault:"" envInfo:"Boltz HTTP endpoint (e.g., http://boltz:9001)"`
	BoltzWSURL string `mapstructure:"BOLTZ_WS_URL" envDefault:"" envInfo:"Boltz WebSocket endpoint (e.g., ws://boltz:9002)"`

	UnlockerType     string `mapstructure:"UNLOCKER_TYPE" envDefault:"" envInfo:"Unlocker type: file | env"`
	UnlockerFilePath string `mapstructure:"UNLOCKER_FILE_PATH" envDefault:"" envInfo:"Path to unlocker file"`
	UnlockerPassword string `mapstructure:"UNLOCKER_PASSWORD" envDefault:"" envInfo:"Unlocker password (if using env unlocker)"`
	DisableTelemetry bool   `mapstructure:"DISABLE_TELEMETRY" envDefault:"false" envInfo:"Disable telemetry"`
	NoMacaroons      bool   `mapstructure:"NO_MACAROONS" envDefault:"false" envInfo:"Disable macaroons"`
	SwapTimeout      uint32 `mapstructure:"SWAP_TIMEOUT" envDefault:"120" envInfo:"Swap timeout in seconds"`

	LndUrl     string `mapstructure:"LND_URL" envDefault:"" envInfo:"LND connection URL (lndconnect:// or http://host:port)"`
	ClnUrl     string `mapstructure:"CLN_URL" envDefault:"" envInfo:"CLN connection URL (clnconnect:// or http://host:port)"`
	ClnDatadir string `mapstructure:"CLN_DATADIR" envDefault:"" envInfo:"CLN data directory (required if not using clnconnect://)"`
	LndDatadir string `mapstructure:"LND_DATADIR" envDefault:"" envInfo:"LND data directory (required if not using lndconnect://)"`

	lnConnectionOpts *domain.LnConnectionOpts

	unlocker    ports.Unlocker
	macaroonSvc macaroon.Service
}

func LoadConfig() (*Config, error) {
	v := viper.New()

	v.SetEnvPrefix("FULMINE")
	v.AutomaticEnv()

	if err := setDefaultConfig(v); err != nil {
		return nil, fmt.Errorf("error setting default config: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %v", err)
	}

	if err := config.initDb(); err != nil {
		return nil, fmt.Errorf("error initializing data directory: %w", err)
	}

	if err := config.deriveLnConfig(); err != nil {
		return nil, fmt.Errorf("error deriving lightning connection config: %w", err)
	}

	if err := config.initUnlockerService(); err != nil {
		return nil, err
	}

	if err := config.initMacaroonService(); err != nil {
		return nil, err
	}

	return &config, nil

}

func (c *Config) deriveLnConfig() error {
	lndUrl := c.LndUrl
	clnUrl := c.ClnUrl
	lndDatadir := cleanAndExpandPath(c.LndDatadir)
	clnDatadir := cleanAndExpandPath(c.ClnDatadir)

	if lndUrl == "" && clnUrl == "" {
		return nil
	}

	if lndUrl != "" && clnUrl != "" {
		return fmt.Errorf("cannot set both LND and CLN URLs at the same time")
	}

	if lndDatadir != "" && clnDatadir != "" {
		return fmt.Errorf("cannot set both LND and CLN datadirs at the same time")
	}

	if lndUrl != "" {
		if strings.HasPrefix(lndUrl, "lndconnect://") {
			c.lnConnectionOpts = &domain.LnConnectionOpts{
				LnUrl:          lndUrl,
				ConnectionType: domain.LND_CONNECTION,
			}

			return nil
		}

		if lndDatadir == "" {
			return fmt.Errorf("LND URL provided without LND datadir")
		}

		if _, err := utils.ValidateURL(lndUrl); err != nil {
			return fmt.Errorf("invalid LND URL: %v", err)
		}
		c.lnConnectionOpts = &domain.LnConnectionOpts{
			LnUrl:          lndUrl,
			LnDatadir:      lndDatadir,
			ConnectionType: domain.LND_CONNECTION,
		}

		return nil
	}

	if strings.HasPrefix(clnUrl, "clnconnect://") {
		c.lnConnectionOpts = &domain.LnConnectionOpts{
			LnUrl:          clnUrl,
			ConnectionType: domain.CLN_CONNECTION,
		}

		return nil
	}

	if clnDatadir == "" {
		return fmt.Errorf("CLN URL provided without CLN datadir")
	}

	if _, err := utils.ValidateURL(clnUrl); err != nil {
		return fmt.Errorf("invalid CLN URL: %v", err)
	}

	c.lnConnectionOpts = &domain.LnConnectionOpts{
		LnUrl:          clnUrl,
		LnDatadir:      clnDatadir,
		ConnectionType: domain.CLN_CONNECTION,
	}

	return nil
}

func (c *Config) UnlockerService() ports.Unlocker {
	return c.unlocker
}

func (c *Config) GetLnConnectionOpts() *domain.LnConnectionOpts {
	return c.lnConnectionOpts
}

func (c *Config) initDb() error {
	supportedDbType := map[string]struct{}{
		sqliteDb: {},
		badgerDb: {},
	}

	if _, ok := supportedDbType[c.DbType]; !ok {
		return fmt.Errorf("unsupported db type: %s", c.DbType)
	}

	if c.Datadir == "fulmine" {
		c.Datadir = appDatadir("fulmine", false)
	} else {
		datadir := viper.GetString(c.Datadir)
		makeDirectoryIfNotExists(datadir)
	}

	return nil

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

	if !c.NoMacaroons {
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

func setDefaultConfig(v *viper.Viper) error {
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		key := f.Tag.Get("mapstructure")
		def := f.Tag.Get("envDefault")
		if def != "" {
			v.SetDefault(key, def)
		}
		err := v.BindEnv(key)
		if err != nil {
			return fmt.Errorf("error binding env variable for key %s: %w", key, err)
		}
	}
	return nil
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

//go:generate go run ../../tools/gen-env-doc/main.go
