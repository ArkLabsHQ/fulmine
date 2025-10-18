package esplora

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Service interface {
	GetBlockHeight(ctx context.Context) (int64, error)
}

type service struct {
	baseUrl        string
	electrumClient *ElectrumClient
	useElectrum    bool
}

// NewService creates a new esplora/electrum service
// If the URL is an Electrum server (host:port format without http/https), it uses Electrum protocol
// Otherwise, it uses the HTTP REST API
func NewService(url string) *service {
	// Check if this is an Electrum server (format: host:port)
	// If it doesn't have http:// or https:// prefix, assume it's Electrum
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		// This is an Electrum server
		return &service{
			baseUrl:        url,
			electrumClient: NewElectrumClient(url, 10*time.Second),
			useElectrum:    true,
		}
	}

	// This is an HTTP REST API (Esplora)
	return &service{
		baseUrl:     url,
		useElectrum: false,
	}
}

func (s *service) GetBlockHeight(ctx context.Context) (int64, error) {
	if s.useElectrum {
		return s.getBlockHeightElectrum(ctx)
	}
	return s.getBlockHeightHTTP(ctx)
}

func (s *service) getBlockHeightElectrum(ctx context.Context) (int64, error) {
	height, err := s.electrumClient.GetBlockchainHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("electrum get height: %w", err)
	}
	return height, nil
}

func (s *service) getBlockHeightHTTP(ctx context.Context) (int64, error) {
	url := strings.TrimRight(s.baseUrl, "/") + "/blocks/tip/height"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
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
