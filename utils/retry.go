package utils

import (
	"context"
	"time"
)

func Retry(ctx context.Context, fn func(ctx context.Context) bool, maxWait time.Duration,
	interval time.Duration) {
	deadline := time.Now().Add(maxWait)

	for {
		if fn(ctx) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		// Sleep before retrying
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
