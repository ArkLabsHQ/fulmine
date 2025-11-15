package monitor

import (
	"context"
	"testing"
	"time"
)

func TestMonitorTracksTaskLifecycle(t *testing.T) {
	mon := New(
		WithStallThreshold(50*time.Millisecond),
		WithCheckInterval(10*time.Millisecond),
	)
	defer mon.Stop()

	handle := mon.Go("test-task", func(ctx context.Context, hb Heartbeat) error {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				hb.Tick()
			}
		}
	})

	// Allow a few heartbeats.
	time.Sleep(20 * time.Millisecond)

	handle.Stop()
	select {
	case <-handle.Done():
	case <-time.After(time.Second):
		t.Fatal("task did not stop in time")
	}

	status := handle.Status()
	if status.State != TaskStateCanceled {
		t.Fatalf("expected canceled state, got %s", status.State)
	}
	if status.HeartbeatStalled {
		t.Fatalf("expected no stall flag")
	}
}
