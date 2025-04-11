package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/go-co-op/gocron"
)

type service struct {
	scheduler *gocron.Scheduler
	job       *gocron.Job
	mu        *sync.Mutex
}

func NewScheduler() ports.SchedulerService {
	svc := gocron.NewScheduler(time.UTC)
	return &service{svc, nil, &sync.Mutex{}}
}

func (s *service) Start() {
	s.scheduler.StartAsync()
}

func (s *service) Stop() {
	s.scheduler.Stop()
}

// ScheduleNextSettlement schedules a Settle() to run in the best market hour
func (s *service) ScheduleNextSettlement(at time.Time, settleFunc func()) error {
	if at.IsZero() {
		return fmt.Errorf("invalid schedule time")
	}

	delay := time.Until(at)
	if delay < 0 {
		return fmt.Errorf("cannot schedule task in the past")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.scheduler.Remove(s.job)
	s.job = nil

	if delay == 0 {
		settleFunc()
		return nil
	}

	job, err := s.scheduler.Every(delay).WaitForSchedule().LimitRunsTo(1).Do(func() {
		settleFunc()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.scheduler.Remove(s.job)
		s.job = nil
	})
	if err != nil {
		return err
	}

	s.job = job

	return err
}

// WhenNextSettlement returns the next scheduled settlement time
func (s *service) WhenNextSettlement() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.job == nil {
		return time.Time{}
	}

	return s.job.NextRun()
}
