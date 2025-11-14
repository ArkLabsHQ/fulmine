# Goroutine Monitor

`pkg/monitor` provides a lightweight supervisor for long-running goroutines. It
tracks heartbeats, catches panics, records failures, and emits warnings when a
task stops sending heartbeats for too long.

Use it for background workers (indexers, polling loops, websocket pumps) where
you need visibility into whether they are still alive without resorting to ad-hoc
logging.

## Quick start

```go
import (
    "context"
    "time"

    "github.com/ArkLabsHQ/fulmine/pkg/monitor"
)

func main() {
    mon := monitor.New(
        monitor.WithStallThreshold(30 * time.Second),
        monitor.WithCheckInterval(5 * time.Second),
    )
    defer mon.Stop()

    mon.Go("price-feed", func(ctx context.Context, hb monitor.Heartbeat) error {
        ticker := time.NewTicker(3 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-ticker.C:
                hb.Tick()                  // report progress
                if err := updatePrice(); err != nil {
                    return err            // failure recorded in monitor
                }
            }
        }
    })

    // ... run the rest of your program ...
}
```

Each task:

1. Receives its own `context.Context`, cancelled when you call `TaskHandle.Stop`
   or `Monitor.Stop`.
2. Receives a `Heartbeat`, call `Tick()` whenever meaningful progress happens.
   Missing heartbeats beyond the stall threshold sets `HeartbeatStalled=true` in
   status snapshots and logs a warning.
3. Should return `nil` on success, `context.Canceled`/`DeadlineExceeded` to mark
   the task as canceled, or any other error to mark it failed. Panics are caught,
   logged, and marked as `panicked`.

## Inspecting status

```go
snapshot := mon.Snapshot()
for _, task := range snapshot.Tasks {
    fmt.Printf("%s -> %s (last heartbeat %s)\n",
        task.Name, task.State, time.Since(task.LastHeartbeat))
}
```

`TaskHandle.Status()` returns the latest snapshot for a single task. This is
useful for exposing metrics (e.g., via Prometheus) or wiring into existing
health endpoints.

## Options

- `WithStallThreshold(duration)` – max time between heartbeats before a warning.
  Default: 2 minutes.
- `WithCheckInterval(duration)` – watchdog inspection cadence. Default: 5
  seconds.
- `WithLogger(logger)` – provide your own logger (implements `Printf`). Defaults
  to the standard library logger.

## Integration tips

- Call `Monitor.Stop()` during shutdown so all tracked goroutines exit and the
  watchdog stops.
- Combine `TaskHandle.Done()` with select statements if you need to await task
  completion.
- The monitor only reports; it will not restart goroutines. If you need
  automatic restarts, wrap failing tasks with your own retry loop but keep the
  monitor to surface stalls and panics.

## Example: Fulmine background loops

`internal/core/application/service.go` now uses the monitor for two long-running
loops:

- `subscribeForBoardingEvent` (`service.go:1398`) runs inside a monitored task
  named `boarding-events`. Every processed utxo update calls `Heartbeat.Tick()`,
  so stalled boarding listeners show up in `/status`.
- `handleAddressEventChannel` (`service.go:1459`) is wrapped by an `address-events`
  monitor task. Each indexer event triggers a heartbeat and any panic or error
  is recorded in the monitor snapshot.
- The settlement scheduler now runs as `scheduler-service`, emitting periodic
  heartbeats while waiting for the next settlement window and shutting the
  scheduler down when the task is canceled (lock/reset).
- LN connectivity is covered by `ln-connector`. Unlocking starts the task in
  “connect + monitor” mode, and manual `ConnectLN` calls restart it in
  heartbeat-only mode. Cancelling the task disconnects from the LN service.

These integrations demonstrate how to retrofit existing goroutines without
rewriting their core logic: wrap the loop with `Monitor.Go`, pass the heartbeat
through, and cancel the task via the returned `TaskHandle` during shutdown.
