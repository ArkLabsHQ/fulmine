package config_test

import (
	"fmt"
	"testing"

	cfg "github.com/ArkLabsHQ/fulmine/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSpecMatchesViperDefaults(t *testing.T) {
	v := viper.New()
	v.SetEnvPrefix("FULMINE")
	v.AutomaticEnv()

	v.SetDefault(cfg.Datadir, cfg.DefaultDatadir)
	v.SetDefault(cfg.GRPCPort, cfg.DefaultGRPCPort)
	v.SetDefault(cfg.HTTPPort, cfg.DefaultHTTPPort)
	v.SetDefault(cfg.WithTLS, cfg.DefaultWithTLS)
	v.SetDefault(cfg.LogLevel, cfg.DefaultLogLevel)
	v.SetDefault(cfg.ArkServer, cfg.DefaultArkServer)
	v.SetDefault(cfg.DisableTelemetry, cfg.DefaultDisableTelemetry)
	v.SetDefault(cfg.DbType, cfg.DefaultDbType)
	v.SetDefault(cfg.NoMacaroons, cfg.DefaultNoMacaroons)
	v.SetDefault(cfg.LndUrl, cfg.DefaultLndUrl)
	v.SetDefault(cfg.ClnUrl, cfg.DefaultClnUrl)
	v.SetDefault(cfg.ClnDatadir, cfg.DefaultClnDatadir)
	v.SetDefault(cfg.LndDatadir, cfg.DefaultLndDatadir)

	want := map[string]string{}
	for _, s := range cfg.EnvSpecs() {
		if s.Default == "" {
			continue
		}
		want[s.Name] = s.Default
	}

	for k, dv := range want {
		got := v.Get(k)
		require.Equal(t, coerce(got), dv, "type mismatch for %s: viper=%T spec=%T", k, got, dv)

	}
}

func coerce(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", x)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	case float32, float64:
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
