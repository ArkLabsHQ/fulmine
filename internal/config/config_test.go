package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelegateCoalesceDefaults(t *testing.T) {
	t.Setenv("DATADIR", t.TempDir())
	c, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, int64(3600), c.DelegateCoalesceWindow)
	require.Equal(t, int64(1800), c.DelegateExpiryMargin)
	require.Equal(t, int64(0), c.DelegateCoalesceMax)
}
