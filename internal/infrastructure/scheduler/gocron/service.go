package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/esplora"
	"github.com/go-co-op/gocron"
)

type heightTask struct {
	target uint32
	fn     func()
}

// TODO: (Joshua) add persistence to survive restarts
type service struct {
	scheduler      *gocron.Scheduler
	esploraService esplora.Service
	job            *gocron.Job
	mu             *sync.Mutex
	blockCancel    context.CancelFunc
	tasks          []*heightTask
}

func NewScheduler(esplorerUrl string) ports.SchedulerService {
	svc := gocron.NewScheduler(time.UTC)
	esplorerService := esplora.NewService(esplorerUrl)
	return &service{svc, esplorerService, nil, &sync.Mutex{}, nil, nil}
}

func (s *service) Start() {
	s.scheduler.StartAsync()

	pollInterval := 5 * time.Second

	s.mu.Lock()
	if s.blockCancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.blockCancel = cancel
	s.mu.Unlock()

	go func() {
		t := time.NewTicker(pollInterval)
		defer t.Stop()
		for {
			callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			h, err := s.esploraService.GetBlockHeight(callCtx)
			cancel()

			if err == nil {
				s.mu.Lock()
				keep := s.tasks[:0]
				for _, tsk := range s.tasks {
					if uint32(h) >= tsk.target {
						go tsk.fn()
						continue
					}
					keep = append(keep, tsk)
				}
				s.tasks = keep
				s.mu.Unlock()
			}

			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *service) Stop() {
	s.scheduler.Stop()

	s.mu.Lock()

	if s.blockCancel != nil {
		s.blockCancel()
		s.blockCancel = nil
	}
	s.mu.Unlock()
}

func (s *service) ScheduleRefundAtHeight(target uint32, refund func()) error {
	if target <= 0 {
		return fmt.Errorf("invalid height: %d", target)
	}
	tsk := &heightTask{target: target, fn: refund}
	s.mu.Lock()
	s.tasks = append(s.tasks, tsk)
	s.mu.Unlock()
	return nil
}

func (s *service) ScheduleRefundAtTime(at time.Time, refundFunc func()) error {
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
		refundFunc()
		return nil
	}

	job, err := s.scheduler.Every(delay).WaitForSchedule().LimitRunsTo(1).Do(func() {
		refundFunc()
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
