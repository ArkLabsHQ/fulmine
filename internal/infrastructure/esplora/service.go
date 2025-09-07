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
	baseUrl string
}

func NewService(url string) *service {
	return &service{
		baseUrl: url,
	}
}

func (s *service) GetBlockHeight(ctx context.Context) (int64, error) {
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
