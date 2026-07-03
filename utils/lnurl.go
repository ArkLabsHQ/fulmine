package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// maxLnurlResponseBytes caps the response body we read; LNURL pay-request and
// invoice responses are tiny, so this prevents a malicious endpoint exhausting
// memory.
const maxLnurlResponseBytes = 64 << 10 // 64 KiB

// lnAddressRe matches a Lightning Address (LUD-16): <local>@<domain> with a TLD.
// Deliberately strict so it doesn't shadow other destination types in the router.
var lnAddressRe = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

// IsLnAddress reports whether s is a Lightning Address (user@domain, LUD-16).
func IsLnAddress(s string) bool {
	return lnAddressRe.MatchString(strings.ToLower(strings.TrimSpace(s)))
}

// IsLnurl reports whether s is a bech32-encoded LNURL (LUD-01), with an optional
// lightning: prefix.
func IsLnurl(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "lightning:")
	return strings.HasPrefix(s, "lnurl1")
}

// IsLnAddressOrLnurl is a convenience check for the send router.
func IsLnAddressOrLnurl(s string) bool {
	return IsLnAddress(s) || IsLnurl(s)
}

type lnurlPayResponse struct {
	Callback       string `json:"callback"`
	MinSendable    int64  `json:"minSendable"` // millisats
	MaxSendable    int64  `json:"maxSendable"` // millisats
	Metadata       string `json:"metadata"`    // JSON-encoded [[type,value],...] array
	CommentAllowed int    `json:"commentAllowed"`
	Tag            string `json:"tag"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
}

type lnurlInvoiceResponse struct {
	Pr     string `json:"pr"` // bolt11 invoice
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// ResolveLightningAddressOrLnurl resolves a Lightning Address (LUD-16) or LNURL-pay
// (LUD-06) into a payable BOLT11 invoice for amountSats.
//
// The destination URL is attacker-controlled, so the default client (used when
// client is nil) enforces https and refuses to connect to loopback/private/
// reserved IP ranges (SSRF protection, applied at dial time so it covers
// redirects and DNS rebinding). Pass a non-nil client only in tests.
func ResolveLightningAddressOrLnurl(client *http.Client, input string, amountSats uint64) (string, error) {
	if client == nil {
		client = newSafeHTTPClient()
	}

	meta, payURL, err := fetchLnurlPayRequest(client, input)
	if err != nil {
		return "", err
	}

	amountMsat := int64(amountSats) * 1000
	if meta.MinSendable > 0 && amountMsat < meta.MinSendable {
		return "", fmt.Errorf("amount below the recipient's minimum (%d sats)", meta.MinSendable/1000)
	}
	if meta.MaxSendable > 0 && amountMsat > meta.MaxSendable {
		return "", fmt.Errorf("amount above the recipient's maximum (%d sats)", meta.MaxSendable/1000)
	}

	cbURL, err := validateOutboundURL(meta.Callback)
	if err != nil {
		return "", fmt.Errorf("invalid callback URL: %w", err)
	}
	// LUD-06: the callback must live on the same host as the pay-request, which
	// also blocks cross-host SSRF pivots to otherwise-public targets.
	if !strings.EqualFold(cbURL.Host, payURL.Host) {
		return "", fmt.Errorf("callback host does not match the Lightning endpoint")
	}
	q := cbURL.Query()
	q.Set("amount", strconv.FormatInt(amountMsat, 10))
	cbURL.RawQuery = q.Encode()

	var inv lnurlInvoiceResponse
	if err := getJSON(client, cbURL.String(), &inv); err != nil {
		return "", fmt.Errorf("failed to fetch the invoice: %w", err)
	}
	if strings.EqualFold(inv.Status, "ERROR") {
		return "", fmt.Errorf("lightning endpoint error: %s", inv.Reason)
	}
	if inv.Pr == "" {
		return "", fmt.Errorf("the Lightning endpoint returned no invoice")
	}
	return inv.Pr, nil
}

// LnurlPayMetadata is the recipient-facing info from an LNURL-pay / Lightning
// Address pay-request, surfaced to the sender before they pick an amount.
type LnurlPayMetadata struct {
	MinSats        uint64
	MaxSats        uint64
	Description    string // the text/plain entry from the LNURL-pay metadata
	CommentAllowed int    // LUD-12 max comment length; 0 if unsupported
}

// ResolveLnurlPayMetadata fetches the pay-request for a Lightning Address (LUD-16)
// or LNURL-pay (LUD-06) and returns its sendable range and description, without
// requesting an invoice. Used to show min/max/description on the send screen.
// Same SSRF protections as ResolveLightningAddressOrLnurl; pass a non-nil client
// only in tests.
func ResolveLnurlPayMetadata(client *http.Client, input string) (*LnurlPayMetadata, error) {
	if client == nil {
		client = newSafeHTTPClient()
	}
	meta, _, err := fetchLnurlPayRequest(client, input)
	if err != nil {
		return nil, err
	}
	return &LnurlPayMetadata{
		MinSats:        msatToSatsCeil(meta.MinSendable),
		MaxSats:        msatToSats(meta.MaxSendable),
		Description:    lnurlDescription(meta.Metadata),
		CommentAllowed: meta.CommentAllowed,
	}, nil
}

// fetchLnurlPayRequest resolves the input to its pay-request URL and fetches the
// LNURL-pay response, validating it's a non-error payRequest. client must be
// non-nil (callers apply newSafeHTTPClient for the nil case). Returns the parsed
// response and the pay-request URL (for the callback same-host check).
func fetchLnurlPayRequest(client *http.Client, input string) (*lnurlPayResponse, *url.URL, error) {
	payURLStr, err := lnurlPayURL(input)
	if err != nil {
		return nil, nil, err
	}
	payURL, err := validateOutboundURL(payURLStr)
	if err != nil {
		return nil, nil, err
	}
	var meta lnurlPayResponse
	if err := getJSON(client, payURL.String(), &meta); err != nil {
		return nil, nil, fmt.Errorf("failed to reach the Lightning endpoint: %w", err)
	}
	if strings.EqualFold(meta.Status, "ERROR") {
		return nil, nil, fmt.Errorf("lightning endpoint error: %s", meta.Reason)
	}
	if !strings.EqualFold(meta.Tag, "payRequest") {
		return nil, nil, fmt.Errorf("not a Lightning pay endpoint")
	}
	return &meta, payURL, nil
}

// msatToSats converts millisats to whole sats, flooring; negatives become 0.
func msatToSats(msat int64) uint64 {
	if msat < 0 {
		return 0
	}
	return uint64(msat) / 1000
}

// msatToSatsCeil converts millisats to whole sats, rounding up; negatives become
// 0. Minimums round up so the advertised value is actually sendable -- the
// backend enforces the exact millisat minimum, so flooring would show a sat
// amount that gets rejected on submit.
func msatToSatsCeil(msat int64) uint64 {
	if msat <= 0 {
		return 0
	}
	return uint64((msat + 999) / 1000)
}

// lnurlDescription extracts the text/plain description from an LNURL-pay metadata
// array ([[type, value], ...]); returns "" if absent or unparseable.
func lnurlDescription(metadata string) string {
	if metadata == "" {
		return ""
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(metadata), &pairs); err != nil {
		return ""
	}
	for _, p := range pairs {
		if len(p) == 2 && p[0] == "text/plain" {
			return p[1]
		}
	}
	return ""
}

// lnurlPayURL turns a Lightning Address or LNURL into its initial pay-request URL.
func lnurlPayURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(strings.TrimPrefix(input, "lightning:"), "LIGHTNING:")

	if IsLnAddress(input) {
		parts := strings.SplitN(strings.ToLower(input), "@", 2)
		return fmt.Sprintf("https://%s/.well-known/lnurlp/%s", parts[1], parts[0]), nil
	}
	if IsLnurl(input) {
		return decodeLnurl(input)
	}
	return "", fmt.Errorf("not a Lightning Address or LNURL")
}

// decodeLnurl bech32-decodes an LNURL into its target URL (LUD-01).
func decodeLnurl(lnurl string) (string, error) {
	lnurl = strings.ToLower(strings.TrimPrefix(strings.ToLower(lnurl), "lightning:"))
	hrp, data, err := bech32.DecodeNoLimit(lnurl)
	if err != nil {
		return "", fmt.Errorf("invalid LNURL: %w", err)
	}
	if hrp != "lnurl" {
		return "", fmt.Errorf("invalid LNURL prefix")
	}
	conv, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return "", fmt.Errorf("invalid LNURL data: %w", err)
	}
	return string(conv), nil
}

// validateOutboundURL parses a destination URL and requires https.
func validateOutboundURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("only https Lightning endpoints are allowed")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	return u, nil
}

// newSafeHTTPClient returns a client that refuses to connect to non-public IPs.
// Dialer.Control runs after DNS resolution with the concrete remote address, so
// it blocks both redirects and DNS-rebinding attempts.
func newSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedIP(ip) {
			return fmt.Errorf("refusing to connect to non-public address %s", host)
		}
		return nil
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

// isBlockedIP reports whether ip is loopback/private/reserved and must not be
// reached by an outbound resolver request.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	// 100.64.0.0/10 (CGNAT) is not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}

func getJSON(client *http.Client, u string, out interface{}) error {
	resp, err := client.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxLnurlResponseBytes)).Decode(out)
}
