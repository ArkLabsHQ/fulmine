package lnurl

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// InvoiceFunc generates a BOLT11 invoice to receive `sats` (fulmine's GetInvoice).
type InvoiceFunc func(ctx context.Context, sats uint64) (string, error)

// DeriveToken returns the stable session token the lnurl-server uses to hand back
// the same LNURL each connection: hex(HMAC-SHA256(privKey, "lnurl-session")).
func DeriveToken(privKey []byte) string {
	mac := hmac.New(sha256.New, privKey)
	mac.Write([]byte("lnurl-session"))
	return hex.EncodeToString(mac.Sum(nil))
}

// Client maintains a persistent lnurl-server session that yields a stable,
// amountless LNURL and bridges incoming pay requests to invoiceFor.
type Client struct {
	baseURL    string
	token      string
	invoiceFor InvoiceFunc

	mu    sync.RWMutex
	lnurl string
}

// New builds a client. privKey is the wallet private key bytes; the token is
// derived from it so the same wallet always gets the same LNURL.
func New(baseURL string, privKey []byte, invoiceFor InvoiceFunc) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      DeriveToken(privKey),
		invoiceFor: invoiceFor,
	}
}

// Lnurl returns the current active LNURL, or "" when no session is established.
func (c *Client) Lnurl() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lnurl
}

func (c *Client) setLnurl(v string) {
	c.mu.Lock()
	c.lnurl = v
	c.mu.Unlock()
}

// Run opens the session and handles events until ctx is cancelled, reconnecting
// with capped backoff after a stream error OR a clean close. Blocks; run it in a
// goroutine.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := c.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.WithError(err).Warn("lnurl: session error, retrying")
		} else {
			log.Debug("lnurl: session ended, reconnecting")
		}
		// Back off before every reconnect, including a clean stream close: a
		// server that returns 200 then EOFs immediately must not spin us in a
		// tight loop. Reset the backoff only after a session that actually
		// stayed up, so a flapping endpoint stays throttled.
		if time.Since(start) > 30*time.Second {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	body := bytes.NewBufferString(fmt.Sprintf(`{"token":%q}`, c.token))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/lnurl/session", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lnurl session open: status %d", resp.StatusCode)
	}

	var sessionID, authToken string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var event string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimSpace(line[7:])
		case strings.HasPrefix(line, "data: ") && event != "":
			data := line[6:]
			switch event {
			case "session_created":
				var d struct {
					SessionId string `json:"sessionId"`
					Token     string `json:"token"`
					Lnurl     string `json:"lnurl"`
				}
				if err := json.Unmarshal([]byte(data), &d); err == nil {
					sessionID, authToken = d.SessionId, d.Token
					c.setLnurl(d.Lnurl)
					log.Infof("lnurl: session established, lnurl=%s", d.Lnurl)
				} else {
					log.WithError(err).Warnf("lnurl: bad session_created data: %s", data)
				}
			case "invoice_request":
				var d struct {
					AmountMsat int64 `json:"amountMsat"`
				}
				if err := json.Unmarshal([]byte(data), &d); err != nil {
					log.WithError(err).Warnf("lnurl: bad invoice_request data: %s", data)
				} else {
					c.handleInvoiceRequest(ctx, sessionID, authToken, d.AmountMsat)
				}
			}
			event = ""
		}
	}
	c.setLnurl("")
	return scanner.Err()
}

func (c *Client) handleInvoiceRequest(ctx context.Context, sessionID, token string, amountMsat int64) {
	// This processes untrusted remote input; contain any panic to this request
	// rather than letting it unwind through the session goroutine and crash the
	// daemon.
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("lnurl: recovered from panic handling an invoice request: %v", r)
		}
	}()
	post := func(payload string) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			fmt.Sprintf("%s/lnurl/session/%s/invoice", c.baseURL, sessionID),
			bytes.NewBufferString(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.WithError(err).Warn("lnurl: failed to post invoice result")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			log.Warnf("lnurl: invoice-result POST returned HTTP %d", resp.StatusCode)
		}
	}
	if amountMsat <= 0 {
		post(`{"error":"invalid amount"}`)
		return
	}
	if amountMsat%1000 != 0 {
		// We mint whole-sat invoices; flooring a sub-sat request would make the
		// invoice amount disagree with what the sender approved, so reject it.
		post(`{"error":"sub-satoshi amounts are not supported"}`)
		return
	}
	pr, err := c.invoiceFor(ctx, uint64(amountMsat)/1000)
	if err != nil || pr == "" {
		reason := "failed to create invoice"
		if err != nil {
			reason = err.Error()
		}
		post(fmt.Sprintf(`{"error":%q}`, reason))
		return
	}
	post(fmt.Sprintf(`{"pr":%q}`, pr))
}
