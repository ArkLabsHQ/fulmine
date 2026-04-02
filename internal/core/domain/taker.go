package domain

import (
	"context"
	"strings"
)

// BancoPair defines a trading pair and its constraints for the banco taker bot.
// The Pair field uses the format "{base}/{quote}" where each side is either
// "BTC" (for native bitcoin) or the hex asset ID (for arkade assets).
// Examples: "a1b2c3.../BTC", "BTC/d4e5f6...", "a1b2c3.../d4e5f6..."
type BancoPair struct {
	Pair          string `json:"pair"`                    // e.g. "a1b2c3.../BTC"
	QuoteAssetID  string `json:"quoteAssetId,omitempty"`  // on-chain asset ID for the quote side (empty = BTC)
	MinAmount     uint64 `json:"minAmount"`               // satoshis
	MaxAmount     uint64 `json:"maxAmount"`               // satoshis
	PriceFeed     string `json:"priceFeed"`               // price API URL
	InvertPrice   bool   `json:"invertPrice"`             // if true, use 1/feedPrice for comparison
}

// Base returns the base asset of the pair (e.g. "BTC" from "BTC/USDT").
func (p BancoPair) Base() string {
	parts := strings.SplitN(p.Pair, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// Quote returns the quote asset of the pair (e.g. "USDT" from "BTC/USDT").
func (p BancoPair) Quote() string {
	parts := strings.SplitN(p.Pair, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// BancoPairRepository persists banco trading pair configuration.
type BancoPairRepository interface {
	Add(ctx context.Context, pair BancoPair) error
	Update(ctx context.Context, pair BancoPair) error
	Remove(ctx context.Context, pairName string) error
	List(ctx context.Context) ([]BancoPair, error)
	Close()
}