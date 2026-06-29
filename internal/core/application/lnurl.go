package application

import (
	"context"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/lnurl"
	log "github.com/sirupsen/logrus"
)

// startLnurlReceiver opens a background lnurl-server session that yields a
// stable, amountless LNURL and bridges incoming pay requests to GetInvoice.
// Started on unlock, cancelled on lock. No-op without a configured lnurl-server
// URL. Safe to call once per unlock.
func (s *Service) startLnurlReceiver() {
	if s.lnurlServerURL == "" {
		return
	}
	if s.privateKey == nil {
		log.Warn("lnurl: configured but wallet key unavailable; receiver not started")
		return
	}
	if s.lnurlCancel != nil {
		// Already running (called from both Setup and a later unlock); restart
		// cleanly rather than leaking a second session.
		s.lnurlCancel()
		s.lnurlClient = nil
	}
	log.Infof("lnurl: starting receiver against %s", s.lnurlServerURL)
	ctx, cancel := context.WithCancel(context.Background())
	s.lnurlCancel = cancel

	invoiceFor := func(ctx context.Context, sats uint64) (string, error) {
		// The swap handler is set up asynchronously on unlock; until then we
		// can't mint invoices, so fail cleanly (the lnurl-server reports it to
		// the payer) rather than nil-panic.
		if s.swapHandler == nil {
			return "", fmt.Errorf("wallet not ready")
		}
		resp, err := s.GetInvoice(ctx, sats)
		if err != nil {
			return "", err
		}
		return resp.Invoice, nil
	}

	c := lnurl.New(s.lnurlServerURL, s.privateKey.Serialize(), invoiceFor)
	s.lnurlClient = c
	go c.Run(ctx)
}

// CurrentLnurl returns the active amountless LNURL, or "" if unavailable
// (no lnurl-server configured, wallet locked, or session not yet established).
func (s *Service) CurrentLnurl() string {
	if s.lnurlClient == nil {
		return ""
	}
	return s.lnurlClient.Lnurl()
}
