package pricefeed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
)

const defaultTimeout = 10 * time.Second

// CoinGeckoPriceFeed fetches prices from a JSON API endpoint.
// It walks the JSON response to find the first numeric value.
// Works with CoinGecko (e.g. https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd)
// and any API returning a JSON object with a numeric price value. // TODO as a workaround
type CoinGeckoPriceFeed struct{}

var _ ports.PriceFeed = (*CoinGeckoPriceFeed)(nil)

func NewCoinGeckoPriceFeed() *CoinGeckoPriceFeed {
	return &CoinGeckoPriceFeed{}
}

func (f *CoinGeckoPriceFeed) FetchPrice(ctx context.Context, feedURL string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("price feed returned status %d: %s", resp.StatusCode, string(body))
	}

	return extractPrice(body)
}

func extractPrice(data []byte) (float64, error) {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}
	price, ok := findFloat(raw)
	if !ok {
		return 0, fmt.Errorf("no numeric value found in response")
	}
	if price <= 0 {
		return 0, fmt.Errorf("invalid price: %f", price)
	}
	return price, nil
}

// TODO fix, that's a workaround
func findFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case map[string]interface{}:
		for _, nested := range val {
			if f, ok := findFloat(nested); ok {
				return f, true
			}
		}
	case []interface{}:
		for _, nested := range val {
			if f, ok := findFloat(nested); ok {
				return f, true
			}
		}
	}
	return 0, false
}
