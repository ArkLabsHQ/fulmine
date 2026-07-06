// internal/core/application/registration_buffer_test.go
package application

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestBuffer returns a buffer with a fixed clock and a no-op timer (tests
// drive flushing by calling flush() via the recorded arm delay explicitly).
func newTestBuffer(window time.Duration, maxBatch int) (*registrationBuffer, *[]string, *time.Duration) {
	registered := &[]string{}
	var armed time.Duration
	b := newRegistrationBuffer(window, maxBatch, func(id string) {
		*registered = append(*registered, id)
	})
	fixed := time.Unix(1_000_000, 0)
	b.now = func() time.Time { return fixed }
	b.arm = func(d time.Duration) { armed = d }
	return b, registered, &armed
}

func TestBufferCoalescesWithinWindow(t *testing.T) {
	b, registered, armed := newTestBuffer(time.Hour, 0)
	far := b.now().Add(24 * time.Hour) // registerBy far away -> window governs
	b.enqueue("a", far)
	require.Equal(t, time.Hour, *armed) // first task opens a 1h window
	b.enqueue("b", far)
	require.Empty(t, *registered) // nothing registered yet
	b.flush()                     // simulate the timer firing
	require.Equal(t, []string{"a", "b"}, *registered)
}

func TestBufferExpiryCapShortensWait(t *testing.T) {
	b, _, armed := newTestBuffer(time.Hour, 0)
	soon := b.now().Add(10 * time.Minute) // registerBy sooner than window
	b.enqueue("a", soon)
	require.Equal(t, 10*time.Minute, *armed)
}

func TestBufferExpiredRegisterByFlushesImmediately(t *testing.T) {
	b, registered, _ := newTestBuffer(time.Hour, 0)
	past := b.now().Add(-time.Minute)
	b.enqueue("a", past) // already past registerBy
	b.flush()
	require.Equal(t, []string{"a"}, *registered)
}

func TestBufferWindowZeroRegistersImmediately(t *testing.T) {
	b, registered, _ := newTestBuffer(0, 0)
	b.enqueue("a", b.now().Add(time.Hour))
	require.Equal(t, []string{"a"}, *registered)
}

func TestBufferMaxBatchFlushesEarly(t *testing.T) {
	b, registered, _ := newTestBuffer(time.Hour, 2)
	far := b.now().Add(24 * time.Hour)
	b.enqueue("a", far)
	require.Empty(t, *registered)
	b.enqueue("b", far) // hits cap of 2 -> flush
	require.Equal(t, []string{"a", "b"}, *registered)
}

func TestBufferFlushNowForces(t *testing.T) {
	b, registered, _ := newTestBuffer(time.Hour, 0)
	far := b.now().Add(24 * time.Hour)
	b.enqueue("a", far)
	b.flushNow()
	require.Equal(t, []string{"a"}, *registered)
}

func TestBufferReopensAfterFlush(t *testing.T) {
	b, registered, armed := newTestBuffer(time.Hour, 0)
	far := b.now().Add(24 * time.Hour)
	b.enqueue("a", far)
	b.flush()
	b.enqueue("c", far)
	require.Equal(t, time.Hour, *armed) // fresh window opened
	require.Equal(t, []string{"a"}, *registered)
}

func TestBufferSnapshot(t *testing.T) {
	b, _, _ := newTestBuffer(time.Hour, 0)
	rb := b.now().Add(24 * time.Hour)
	b.enqueue("a", rb)
	entries, nextFlush := b.snapshot()
	require.Len(t, entries, 1)
	require.Equal(t, "a", entries[0].ID)
	require.Equal(t, b.now().Add(time.Hour), nextFlush)
}
