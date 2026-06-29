package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// LoadConfig hand-assembles Config field-by-field from viper keys: a field
// added to the struct without a matching key constant AND assembly line stays
// silently empty (the env-doc generator only reads struct tags, so it won't
// catch the gap). This locks in the env->Config read for LNURL_SERVER_URL.
func TestLoadConfigReadsLnurlServerURL(t *testing.T) {
	t.Setenv("FULMINE_DATADIR", t.TempDir())
	t.Setenv("FULMINE_LNURL_SERVER_URL", "http://lnurl-server:3000")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "http://lnurl-server:3000", cfg.LnurlServerURL)
}
