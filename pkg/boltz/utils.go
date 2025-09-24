package boltz

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func Retry(
	ctx context.Context, interval time.Duration, fn func(ctx context.Context) (bool, error),
) error {
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out")
			}
			return ctx.Err()
		default:
			done, err := fn(ctx)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			<-time.After(interval)
		}
	}
}
