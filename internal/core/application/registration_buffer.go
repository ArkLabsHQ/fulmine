package application

import (
	"sync"
	"time"
)

// bufferedRegistration is a delegate task waiting to be registered.
type bufferedRegistration struct {
	ID         string
	RegisterBy time.Time
}

// registrationBuffer coalesces ready delegate tasks so intents that become
// registrable close together are registered as a group (landing in one server
// round). The group flushes at the earliest of: the coalescing window measured
// from the first buffered task, or the earliest per-task RegisterBy.
type registrationBuffer struct {
	mu       sync.Mutex
	window   time.Duration
	maxBatch int             // <= 0 means unbounded
	register func(id string) // registers a single task by id (best-effort)

	now func() time.Time      // clock (injectable for tests)
	arm func(d time.Duration) // (re)arm the flush timer (injectable for tests)

	entries  []bufferedRegistration
	openedAt time.Time
}

func newRegistrationBuffer(
	window time.Duration, maxBatch int, register func(id string),
) *registrationBuffer {
	b := &registrationBuffer{
		window:   window,
		maxBatch: maxBatch,
		register: register,
		now:      time.Now,
	}
	var timer *time.Timer
	b.arm = func(d time.Duration) {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(d, b.flush)
	}
	return b
}

// enqueue adds a ready task. registerBy is the latest time it may be registered
// (min input ExpiresAt minus the expiry margin).
func (b *registrationBuffer) enqueue(id string, registerBy time.Time) {
	b.mu.Lock()
	if len(b.entries) == 0 {
		b.openedAt = b.now()
	}
	b.entries = append(b.entries, bufferedRegistration{ID: id, RegisterBy: registerBy})

	if b.window <= 0 || (b.maxBatch > 0 && len(b.entries) >= b.maxBatch) {
		ids := b.drainLocked()
		b.mu.Unlock()
		b.registerAll(ids)
		return
	}
	b.arm(b.nextFlushDelayLocked())
	b.mu.Unlock()
}

// flush drains and registers everything. Registration runs outside the lock so
// a slow RegisterIntent cannot block enqueue/snapshot.
func (b *registrationBuffer) flush() {
	b.mu.Lock()
	ids := b.drainLocked()
	b.mu.Unlock()
	b.registerAll(ids)
}

// flushNow forces an immediate flush regardless of the window.
func (b *registrationBuffer) flushNow() { b.flush() }

// snapshot returns a copy of buffered entries and the projected next flush time.
func (b *registrationBuffer) snapshot() ([]bufferedRegistration, time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := make([]bufferedRegistration, len(b.entries))
	copy(entries, b.entries)
	var nextFlush time.Time
	if len(b.entries) > 0 {
		nextFlush = b.now().Add(b.nextFlushDelayLocked())
	}
	return entries, nextFlush
}

func (b *registrationBuffer) nextFlushDelayLocked() time.Duration {
	deadline := b.openedAt.Add(b.window)
	for _, e := range b.entries {
		if e.RegisterBy.Before(deadline) {
			deadline = e.RegisterBy
		}
	}
	if d := deadline.Sub(b.now()); d > 0 {
		return d
	}
	return 0
}

func (b *registrationBuffer) drainLocked() []string {
	if len(b.entries) == 0 {
		return nil
	}
	ids := make([]string, len(b.entries))
	for i, e := range b.entries {
		ids[i] = e.ID
	}
	b.entries = nil
	b.openedAt = time.Time{}
	return ids
}

func (b *registrationBuffer) registerAll(ids []string) {
	for _, id := range ids {
		b.register(id)
	}
}
