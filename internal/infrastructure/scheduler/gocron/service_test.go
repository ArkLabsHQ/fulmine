package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerService(t *testing.T) {
	t.Run("Schedule Next Settlement", func(t *testing.T) {
		svc := NewScheduler("")
		svc.Start()
		defer svc.Stop()

		// Test scheduling in the future
		done := make(chan bool)
		settleFunc := func() {
			go func() {
				done <- true
			}()
		}

		// Schedule 5 second in the future
		nextTime := time.Now().Add(5 * time.Second)
		now := time.Now()
		err := svc.ScheduleNextSettlement(nextTime, settleFunc)
		require.NoError(t, err)

		// Verify next settlement time
		nextSettlement := svc.WhenNextSettlement()
		require.False(t, nextSettlement.IsZero())
		require.True(t, nextSettlement.After(now))
		require.True(t, nextSettlement.Before(now.Add(5*time.Second).Add(1*time.Millisecond)))

		// Wait for the job to execute
		select {
		case <-done:
			require.True(t, svc.WhenNextSettlement().IsZero())
		case <-time.After(10 * time.Second):
			require.Fail(t, "job did not execute within expected time")
		}

		// verify it won't run again
		select {
		case <-done:
			require.Fail(t, "job executed again")
		case <-time.After(10 * time.Second):
			// Job did not execute again
		}

	})

	t.Run("Schedule in Past", func(t *testing.T) {
		svc := NewScheduler("")
		svc.Start()
		defer svc.Stop()

		executed := false
		settleFunc := func() {
			executed = true
		}

		// Try to schedule in the past
		pastTime := time.Now().Add(-1 * time.Hour)
		err := svc.ScheduleNextSettlement(pastTime, settleFunc)
		require.Error(t, err)
		require.False(t, executed)
	})

	t.Run("Schedule Immediate Execution", func(t *testing.T) {
		svc := NewScheduler("")
		svc.Start()
		defer svc.Stop()

		done := make(chan bool)
		settleFunc := func() {
			done <- true
		}

		// Schedule for immediate execution (add a small buffer to ensure it's not considered past)
		err := svc.ScheduleNextSettlement(time.Now().Add(100*time.Millisecond), settleFunc)
		require.NoError(t, err)

		select {
		case <-done:
			// Job executed successfully
		case <-time.After(1 * time.Second):
			require.Fail(t, "job did not execute within expected time")
		}
	})
}
