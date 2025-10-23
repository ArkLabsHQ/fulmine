package scheduler_test

import (
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	scheduler "github.com/ArkLabsHQ/fulmine/internal/infrastructure/scheduler/gocron"
	"github.com/stretchr/testify/require"
)

const pollInterval = 5 * time.Second

var schedulerTypes = map[string]func() ports.SchedulerService{
	"gocron": func() ports.SchedulerService {
		return scheduler.NewScheduler("", pollInterval)
	},
}

func TestSchedulerService(t *testing.T) {
	for schedulerType, factory := range schedulerTypes {
		t.Run(schedulerType, func(t *testing.T) {
			testScheduler(t, factory)
		})
	}
}

func testScheduler(t *testing.T, newScheduler func() ports.SchedulerService) {
	t.Run("schedule next settlement", func(t *testing.T) {
		svc := newScheduler()
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

	t.Run("schedule settlement in the past", func(t *testing.T) {
		svc := newScheduler()
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

	t.Run("schedule settlement for now", func(t *testing.T) {
		svc := newScheduler()
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
	t.Run("cancel next settlement", func(t *testing.T) {
		svc := newScheduler()
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

		time.Sleep(time.Second)

		svc.CancelNextSettlement()
		nextSettlement = svc.WhenNextSettlement()
		require.True(t, nextSettlement.IsZero())

		// Wait for the job to execute
		select {
		case <-done:
			require.Fail(t, "job shouldn't have been executed")
		case <-time.After(10 * time.Second):
			// Job did not execute because it was indeed cancelled.
		}
	})
}
