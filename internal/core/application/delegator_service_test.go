package application

import (
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/stretchr/testify/require"
)

func TestEarliestInputExpiry(t *testing.T) {
	t1 := time.Unix(2000, 0)
	t2 := time.Unix(1000, 0) // earliest
	t3 := time.Unix(3000, 0)
	got, err := earliestInputExpiry([]clientTypes.Vtxo{
		{ExpiresAt: t1}, {ExpiresAt: t2}, {ExpiresAt: t3},
	})
	require.NoError(t, err)
	require.Equal(t, t2, got)
}

func TestEarliestInputExpiryEmpty(t *testing.T) {
	_, err := earliestInputExpiry(nil)
	require.Error(t, err)
}

func TestDelegateServiceEnqueueUsesExpiryMargin(t *testing.T) {
	// Build a bare DelegateService with just the buffer wired, bypassing svc.
	registered := []string{}
	margin := 30 * time.Minute
	s := &DelegateService{expiryMargin: margin}
	s.registrationBuffer = newRegistrationBuffer(time.Hour, 0, func(id string) {
		registered = append(registered, id)
	})
	var armed time.Duration
	s.registrationBuffer.now = func() time.Time { return time.Unix(1_000_000, 0) }
	s.registrationBuffer.arm = func(d time.Duration) { armed = d }

	// input expires in 40m; registerBy = 40m - 30m margin = 10m from now.
	task := &domain.DelegateTask{
		ID:                     "t1",
		EarliestInputExpiresAt: s.registrationBuffer.now().Add(40 * time.Minute),
	}
	s.enqueueForRegistration(task)
	require.Equal(t, 10*time.Minute, armed)
}

// TestDelegateServiceEnqueueZeroExpiryRegistersImmediately documents the
// money-safety fail-safe: a task whose expiry could not be resolved (zero
// EarliestInputExpiresAt) must register immediately rather than being held,
// since registerBy ends up far in the past.
func TestDelegateServiceEnqueueZeroExpiryRegistersImmediately(t *testing.T) {
	registered := []string{}
	margin := 30 * time.Minute
	s := &DelegateService{expiryMargin: margin}
	s.registrationBuffer = newRegistrationBuffer(time.Hour, 0, func(id string) {
		registered = append(registered, id)
	})
	var armed time.Duration
	s.registrationBuffer.now = func() time.Time { return time.Unix(1_000_000, 0) }
	s.registrationBuffer.arm = func(d time.Duration) { armed = d }

	task := &domain.DelegateTask{
		ID: "t2",
		// EarliestInputExpiresAt left zero-value: expiry could not be resolved.
	}
	s.enqueueForRegistration(task)
	require.Equal(t, time.Duration(0), armed)
}
