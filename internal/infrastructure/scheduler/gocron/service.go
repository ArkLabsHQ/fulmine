package scheduler

import (
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/go-co-op/gocron"
)

type service struct {
	scheduler *gocron.Scheduler
	job       *gocron.Job
}

func NewScheduler() ports.SchedulerService {
	svc := gocron.NewScheduler(time.UTC)
	return &service{svc, nil}
}

func (s *service) Start() {
	s.scheduler.StartAsync()
}

func (s *service) Stop() {
	s.scheduler.Stop()
}

// ScheduleNextSettlement schedules a Settle() to run in the best market hour
func (s *service) ScheduleNextSettlement(at time.Time, cfg *types.Config, settleFunc func()) error {
	roundInterval := time.Duration(cfg.RoundInterval) * time.Second
	at = at.Add(-2 * roundInterval) // schedule 2 rounds before the expiry

	delay := time.Until(at)
	if delay < 0 {
		return fmt.Errorf("cannot schedule task in the past")
	}

	s.scheduler.Remove(s.job)

	if delay == 0 {
		settleFunc()
		return nil
	}

	job, err := s.scheduler.Every(int(delay)).Seconds().WaitForSchedule().LimitRunsTo(1).Do(settleFunc)
	if err != nil {
		return err
	}

	s.job = job

	return err
}

// WhenNextSettlement returns the next scheduled settlement time
func (s *service) WhenNextSettlement() time.Time {
	if s.job == nil {
		return time.Time{}
	}

	return s.job.NextRun()
}
