package utils

import (
	"fmt"
	"net"
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

	t.Run("rejects amount below the recipient minimum", func(t *testing.T) {
		// srv advertises minSendable 1000 msat (1 sat); 0 sats is below it.
		_, err := ResolveLightningAddressOrLnurl(srv.Client(), lnurl, 0)
		require.ErrorContains(t, err, "minimum")
	})

	t.Run("rejects a cross-host callback (LUD-06 same-host rule)", func(t *testing.T) {
		xMux := http.NewServeMux()
		xMux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"tag":"payRequest","callback":"https://example.com/cb","minSendable":1000,"maxSendable":100000000}`)
		})
		xSrv := httptest.NewTLSServer(xMux)
		defer xSrv.Close()
		_, err := ResolveLightningAddressOrLnurl(xSrv.Client(), encodeLnurl(t, xSrv.URL+"/pay"), 1000)
		require.ErrorContains(t, err, "callback host")
	})

	t.Run("rejects a missing invoice in the callback response", func(t *testing.T) {
		emptyMux := http.NewServeMux()
		var emptyURL string
		emptyMux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"tag":"payRequest","callback":%q,"minSendable":1000,"maxSendable":100000000}`, emptyURL+"/cb")
		})
		emptyMux.HandleFunc("/cb", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"OK"}`) // no pr field
		})
		emptySrv := httptest.NewTLSServer(emptyMux)
		defer emptySrv.Close()
		emptyURL = emptySrv.URL
		_, err := ResolveLightningAddressOrLnurl(emptySrv.Client(), encodeLnurl(t, emptySrv.URL+"/pay"), 1000)
		require.ErrorContains(t, err, "no invoice")
	})

	t.Run("rejects a malformed pay-request body", func(t *testing.T) {
		badMux := http.NewServeMux()
		badMux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `<html>definitely not json</html>`)
		})
		badSrv := httptest.NewTLSServer(badMux)
		defer badSrv.Close()
		_, err := ResolveLightningAddressOrLnurl(badSrv.Client(), encodeLnurl(t, badSrv.URL+"/pay"), 1000)
		require.ErrorContains(t, err, "failed to reach")
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

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"10.0.0.1", "192.168.1.1", "172.16.0.1", "fc00::1", // private
		"169.254.0.1", "fe80::1", // link-local
		"0.0.0.0", "::", // unspecified
		"224.0.0.1", "ff02::1", // multicast
		"100.64.0.1", "100.127.255.255", // CGNAT 100.64.0.0/10
	}
	for _, s := range blocked {
		require.True(t, isBlockedIP(net.ParseIP(s)), s)
	}
	public := []string{
		"8.8.8.8", "1.1.1.1", "203.0.113.10", "2606:4700:4700::1111",
		"100.63.255.255", "100.128.0.0", // just outside the CGNAT range
	}
	for _, s := range public {
		require.False(t, isBlockedIP(net.ParseIP(s)), s)
	}
}

func TestLnurlPayURL(t *testing.T) {
	t.Run("lightning address maps to the LUD-16 well-known URL", func(t *testing.T) {
		u, err := lnurlPayURL("Alice@Example.com")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/.well-known/lnurlp/alice", u)
	})
	t.Run("strips a lightning: prefix", func(t *testing.T) {
		u, err := lnurlPayURL("lightning:bob@example.com")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/.well-known/lnurlp/bob", u)
	})
	t.Run("decodes an LNURL to its target URL", func(t *testing.T) {
		u, err := lnurlPayURL(encodeLnurl(t, "https://example.com/pay"))
		require.NoError(t, err)
		require.Equal(t, "https://example.com/pay", u)
	})
	t.Run("rejects a non-address, non-LNURL input", func(t *testing.T) {
		_, err := lnurlPayURL("not-a-destination")
		require.Error(t, err)
	})
}

func TestResolveLnurlPayMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag":"payRequest","callback":"https://x/cb","minSendable":1000,"maxSendable":500000000,"metadata":"[[\"text/plain\",\"Pay Alice\"],[\"text/identifier\",\"alice@x.com\"]]","commentAllowed":120}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	t.Run("returns min/max sats, description and comment length", func(t *testing.T) {
		meta, err := ResolveLnurlPayMetadata(srv.Client(), encodeLnurl(t, srv.URL+"/pay"))
		require.NoError(t, err)
		require.Equal(t, uint64(1), meta.MinSats)      // 1000 msat -> 1 sat
		require.Equal(t, uint64(500000), meta.MaxSats) // 500_000_000 msat -> 500_000 sat
		require.Equal(t, "Pay Alice", meta.Description)
		require.Equal(t, 120, meta.CommentAllowed)
	})

	t.Run("surfaces an endpoint error", func(t *testing.T) {
		errMux := http.NewServeMux()
		errMux.HandleFunc("/pay", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"ERROR","reason":"no such user"}`)
		})
		errSrv := httptest.NewTLSServer(errMux)
		defer errSrv.Close()
		_, err := ResolveLnurlPayMetadata(errSrv.Client(), encodeLnurl(t, errSrv.URL+"/pay"))
		require.ErrorContains(t, err, "no such user")
	})
}

func TestLnurlDescription(t *testing.T) {
	require.Equal(t, "Hello", lnurlDescription(`[["text/plain","Hello"],["image/png;base64","x"]]`))
	require.Equal(t, "", lnurlDescription(""))
	require.Equal(t, "", lnurlDescription(`not json`))
	require.Equal(t, "", lnurlDescription(`[["text/identifier","a@b.com"]]`)) // no text/plain
}
