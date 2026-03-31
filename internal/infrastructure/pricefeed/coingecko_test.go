package pricefeed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/pricefeed"
	"github.com/stretchr/testify/require"
)

func TestCoinGeckoFetchPrice(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		statusCode  int
		expected    float64
		expectError bool
	}{
		{
			name:       "coingecko nested format",
			response:   `{"bitcoin":{"usd":50000.5}}`,
			statusCode: http.StatusOK,
			expected:   50000.5,
		},
		{
			name:       "flat format",
			response:   `{"price": 42000}`,
			statusCode: http.StatusOK,
			expected:   42000,
		},
		{
			name:        "non-200 status",
			response:    `{"error":"rate limited"}`,
			statusCode:  http.StatusTooManyRequests,
			expectError: true,
		},
		{
			name:        "invalid json",
			response:    `not json`,
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name:        "no numeric value",
			response:    `{"status":"ok"}`,
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name:        "negative price",
			response:    `{"price": -1}`,
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name:        "zero price",
			response:    `{"price": 0}`,
			statusCode:  http.StatusOK,
			expectError: true,
		},
	}

	feed := pricefeed.NewCoinGeckoPriceFeed()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprintln(w, tt.response)
			}))
			defer srv.Close()

			price, err := feed.FetchPrice(t.Context(), srv.URL)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, tt.expected, price, 0.01)
		})
	}
}

func TestCoinGeckoFetchPriceCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"price": 100}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	feed := pricefeed.NewCoinGeckoPriceFeed()
	_, err := feed.FetchPrice(ctx, srv.URL)
	require.Error(t, err)
}
