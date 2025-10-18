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
// If electrumURL is provided, it uses Electrum protocol
// Otherwise, it falls back to HTTP REST API with esploraURL
func NewService(esploraURL, electrumURL string) *service {
	// Prioritize Electrum if provided
	if electrumURL != "" {
		return &service{
			baseUrl:        electrumURL,
			electrumClient: NewElectrumClient(electrumURL, 10*time.Second),
			useElectrum:    true,
		}
	}

	// Fall back to HTTP REST API (Esplora)
	return &service{
		baseUrl:     esploraURL,
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
