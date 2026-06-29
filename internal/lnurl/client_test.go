package lnurl

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDeriveToken(t *testing.T) {
	key := []byte{1, 2, 3, 4}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("lnurl-session"))
	want := hex.EncodeToString(mac.Sum(nil))

	got := DeriveToken(key)
	if got != want {
		t.Fatalf("DeriveToken = %s, want %s", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(got))
	}
}

func TestRunHandlesSessionAndInvoice(t *testing.T) {
	var gotPR string
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lnurl/session" && r.Method == http.MethodPost:
			fl, _ := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: session_created\ndata: {\"sessionId\":\"sess1\",\"token\":\"tok1\",\"lnurl\":\"lnurl1xyz\"}\n\n")
			fl.Flush()
			fmt.Fprint(w, "event: invoice_request\ndata: {\"amountMsat\":50000}\n\n")
			fl.Flush()
			<-r.Context().Done()
		case r.URL.Path == "/lnurl/session/sess1/invoice" && r.Method == http.MethodPost:
			var body struct {
				Pr string `json:"pr"`
			}
			_ = json.NewDecoder(bufio.NewReader(r.Body)).Decode(&body)
			mu.Lock()
			gotPR = body.Pr
			mu.Unlock()
			if body.Pr != "" {
				close(done)
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	invoiceFor := func(ctx context.Context, sats uint64) (string, error) {
		if sats != 50 {
			t.Errorf("sats = %d, want 50 (50000 msat / 1000)", sats)
		}
		return "lnbcSIMULATED", nil
	}
	c := New(srv.URL, []byte{9, 9}, invoiceFor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("invoice was not posted in time")
	}

	// The session is still active here (server holds the stream open), so the
	// LNURL is set; check before cancelling to avoid the disconnect cleanup.
	if c.Lnurl() != "lnurl1xyz" {
		t.Fatalf("Lnurl = %q, want lnurl1xyz", c.Lnurl())
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPR != "lnbcSIMULATED" {
		t.Fatalf("posted pr = %q", gotPR)
	}
}

func TestHandleInvoiceRequestRecoversFromPanic(t *testing.T) {
	// A panicking invoiceFor (e.g. the historical nil-deref on an out-of-range
	// amount) must be contained to the request, not propagate through the session
	// goroutine and crash the daemon.
	c := New("http://lnurl-server.invalid", []byte{1, 2, 3, 4}, func(context.Context, uint64) (string, error) {
		panic("boom")
	})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleInvoiceRequest let a panic escape: %v", r)
		}
	}()
	c.handleInvoiceRequest(context.Background(), "sess", "tok", 50000)
}

func TestRunBackoffOnCleanClose(t *testing.T) {
	// A server that returns 200 then closes the stream cleanly (no events) must
	// not spin Run in a tight reconnect loop -- the backoff applies to clean
	// closes too, not only errors.
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusOK) // 200, then an immediate clean EOF
	}))
	defer srv.Close()

	c := New(srv.URL, []byte{1, 2, 3, 4}, func(context.Context, uint64) (string, error) { return "", nil })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.Run(ctx) // blocks until ctx is done

	mu.Lock()
	n := attempts
	mu.Unlock()
	if n > 5 {
		t.Fatalf("clean-close reconnect not throttled: %d connect attempts in 2s", n)
	}
}
