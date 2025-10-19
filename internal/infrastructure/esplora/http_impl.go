package esplora

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// httpService implements the Service interface using HTTP REST API (Esplora)
type httpService struct {
	baseURL string
	client  *http.Client
}

// NewHTTPService creates a new HTTP-based blockchain service (Esplora)
func NewHTTPService(url string) Service {
	return &httpService{
		baseURL: url,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *httpService) GetBlockHeight(ctx context.Context) (int64, error) {
	url := strings.TrimRight(s.baseURL, "/") + "/blocks/tip/height"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("get height: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse height: %w", err)
	}
	return n, nil
}

// GetScriptHashHistory retrieves the transaction history for an address (HTTP doesn't use script hash)
func (s *httpService) GetScriptHashHistory(ctx context.Context, address string) ([]TransactionItem, error) {
	url := strings.TrimRight(s.baseURL, "/") + "/address/" + address + "/txs"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get address txs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var txs []struct {
		Txid   string `json:"txid"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height"`
		} `json:"status"`
		Fee int64 `json:"fee"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&txs); err != nil {
		return nil, fmt.Errorf("failed to parse transactions: %w", err)
	}

	items := make([]TransactionItem, len(txs))
	for i, tx := range txs {
		items[i] = TransactionItem{
			Height: tx.Status.BlockHeight,
			TxHash: tx.Txid,
			Fee:    tx.Fee,
		}
	}

	return items, nil
}

// SubscribeScriptHash is not supported for HTTP API
func (s *httpService) SubscribeScriptHash(ctx context.Context, address string) (string, error) {
	return "", fmt.Errorf("subscription not supported for HTTP API")
}

// Close closes any resources (no-op for HTTP)
func (s *httpService) Close() error {
	return nil
}
