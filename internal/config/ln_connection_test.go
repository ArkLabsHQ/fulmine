package config

import (
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/stretchr/testify/require"
)

func TestLnConnectionOpts(t *testing.T) {
	t.Run("basic constraints", testBasicConstraints)
	t.Run("lnd with port", testLNDWithPort)
	t.Run("cln with port", testCLNWithPort)
	t.Run("lnconnect URLs", testLnConnectURLs)
}

func testBasicConstraints(t *testing.T) {
	_, err := deriveLnConfig("", "", "", "")
	require.NoError(t, err)

	_, err = deriveLnConfig("lnd", "cln", "/tmp/lnd", "/tmp/cln")
	require.ErrorContains(t, err, "cannot set both LND and CLN URLs")

	_, err = deriveLnConfig("http://localhost:10009", "", "", "")
	require.ErrorContains(t, err, "LND URL provided without LND datadir")

	_, err = deriveLnConfig("", "https://localhost:10009", "", "")
	require.ErrorContains(t, err, "CLN URL provided without CLN datadir")

	_, err = deriveLnConfig("http://localhost:10009", "", "/tmp/lnd", "/tmp/cln")
	require.ErrorContains(t, err, "cannot set both LND and CLN datadirs")
}

func testLNDWithPort(t *testing.T) {
	opts, err := deriveLnConfig("http://localhost:10501", "", "/tmp/lnd", "")
	require.NoError(t, err)
	require.NotNil(t, opts)
	require.Equal(t, domain.LND_CONNECTION, opts.ConnectionType)
	require.Equal(t, "localhost:10501", opts.LnUrl)
}

func testCLNWithPort(t *testing.T) {
	opts, err := deriveLnConfig("", "https://example.com:11001", "", "/tmp/cln")
	require.NoError(t, err)
	require.NotNil(t, opts)
	require.Equal(t, domain.CLN_CONNECTION, opts.ConnectionType)
	require.Equal(t, "example.com:11001", opts.LnUrl)
}

func testLnConnectURLs(t *testing.T) {
	lndOpts, err := deriveLnConfig("lndconnect://abc", "", "", "")
	require.NoError(t, err)
	require.Equal(t, &domain.LnConnectionOpts{LnUrl: "lndconnect://abc", ConnectionType: domain.LND_CONNECTION}, lndOpts)

	clnOpts, err := deriveLnConfig("", "clnconnect://def", "", "")
	require.NoError(t, err)
	require.Equal(t, &domain.LnConnectionOpts{LnUrl: "clnconnect://def", ConnectionType: domain.CLN_CONNECTION}, clnOpts)
}
