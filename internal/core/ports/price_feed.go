package ports

import "context"

// PriceFeed fetches asset prices from an external source.
type PriceFeed interface {
	// FetchPrice fetches the current price from the given feed URL.
	FetchPrice(ctx context.Context, feedURL string) (float64, error)
}
