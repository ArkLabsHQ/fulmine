package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/stretchr/testify/require"
)

func TestIsLnAddress(t *testing.T) {
	for _, s := range []string{"alice@example.com", "bob.smith@walletofsatoshi.com", "x@a.io", "ALICE@Example.com"} {
		require.True(t, IsLnAddress(s), s)
	}
	for _, s := range []string{"", "alice", "alice@", "@example.com", "alice@localhost", "lnbc1abc", "bc1qxyz", "alice@exam ple.com"} {
		require.False(t, IsLnAddress(s), s)
	}
}

func TestIsLnurl(t *testing.T) {
	for _, s := range []string{"lnurl1dp68gurn8ghj7", "LNURL1DP68GURN8GHJ7", "lightning:lnurl1dp68"} {
		require.True(t, IsLnurl(s), s)
	}
	for _, s := range []string{"", "alice@example.com", "lnbc1abc", "bc1qxyz"} {
		require.False(t, IsLnurl(s), s)
	}
}

// encodeLnurl bech32-encodes a (short) URL into an lnurl1… string for tests.
func encodeLnurl(t *testing.T, rawURL string) string {
	t.Helper()
	conv, err := bech32.ConvertBits([]byte(rawURL), 8, 5, true)
	require.NoError(t, err)
	s, err := bech32.Encode("lnurl", conv)
	require.NoError(t, err)
	return strings.ToUpper(s)
}

func TestResolveLightningAddressOrLnurl(t *testing.T) {
	const wantInvoice = "lnbc10n1pjqfakeinvoice"

	mux := http.NewServeMux()
	var serverURL string
	mux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag":"payRequest","callback":%q,"minSendable":1000,"maxSendable":100000000}`, serverURL+"/cb")
	})
	mux.HandleFunc("/cb", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("amount") == "" {
			http.Error(w, "missing amount", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"pr":%q,"status":"OK"}`, wantInvoice)
	})
	// HTTPS so it passes the resolver's https requirement; srv.Client() trusts the
	// test cert and is a plain client (no SSRF dial guard), so loopback is allowed.
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	serverURL = srv.URL
	lnurl := encodeLnurl(t, srv.URL+"/pay")

	t.Run("resolves LNURL to an invoice", func(t *testing.T) {
		inv, err := ResolveLightningAddressOrLnurl(srv.Client(), lnurl, 1000)
		require.NoError(t, err)
		require.Equal(t, wantInvoice, inv)
	})

	t.Run("rejects amount above the recipient maximum", func(t *testing.T) {
		_, err := ResolveLightningAddressOrLnurl(srv.Client(), lnurl, 1_000_000)
		require.ErrorContains(t, err, "maximum")
	})

	t.Run("surfaces an endpoint error status", func(t *testing.T) {
		errMux := http.NewServeMux()
		errMux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"ERROR","reason":"unknown user"}`)
		})
		errSrv := httptest.NewTLSServer(errMux)
		defer errSrv.Close()
		_, err := ResolveLightningAddressOrLnurl(errSrv.Client(), encodeLnurl(t, errSrv.URL+"/pay"), 1000)
		require.ErrorContains(t, err, "unknown user")
	})
}

func TestResolveRejectsSSRFTargets(t *testing.T) {
	t.Run("blocks loopback / private IPs via the default safe client", func(t *testing.T) {
		// nil client => the hardened default with the dial-time IP guard.
		_, err := ResolveLightningAddressOrLnurl(nil, encodeLnurl(t, "https://127.0.0.1:1/pay"), 1000)
		require.ErrorContains(t, err, "non-public")
	})

	t.Run("rejects non-https endpoints", func(t *testing.T) {
		_, err := ResolveLightningAddressOrLnurl(nil, encodeLnurl(t, "http://example.com/pay"), 1000)
		require.ErrorContains(t, err, "https")
	})
}
