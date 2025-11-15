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

type service struct {
	scheduler            *gocron.Scheduler
	explorer             esplora.Service
	job                  *gocron.Job
	stopJob              func()
	mu                   *sync.Mutex
	blockCancel          context.CancelFunc
	tasks                []*heightTask
	explorerPollInterval time.Duration
	callbacks            ports.SchedulerCallbacks
}

func NewScheduler(esplorerUrl string, pollInterval time.Duration) ports.SchedulerService {
	svc := gocron.NewScheduler(time.UTC)
	esplorerService := esplora.NewService(esplorerUrl)
	return &service{svc, esplorerService, nil, nil, &sync.Mutex{}, nil, nil, pollInterval, ports.SchedulerCallbacks{}}
}

func (s *service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Nothing to do if already started
	if s.blockCancel != nil {
		return
	}

	s.scheduler.StartAsync()

	ctx, cancel := context.WithCancel(context.Background())
	s.blockCancel = cancel

	go func() {
		t := time.NewTicker(s.explorerPollInterval)
		defer t.Stop()
		for {
			h, err := s.explorer.GetBlockHeight(ctx)

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
				if cb := s.callbacks.OnHeartbeat; cb != nil {
					cb()
				}
				s.mu.Unlock()
			} else {
				if cb := s.callbacks.OnError; cb != nil {
					cb(err)
				}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.scheduler != nil {
		s.scheduler.Remove(s.job)
		s.scheduler.Stop()
		s.scheduler.Clear()

		s.job = nil
		s.tasks = make([]*heightTask, 0)
		// Without this
		// s.scheduler = gocron.NewScheduler(time.UTC)
	}

	if s.blockCancel != nil {
		s.blockCancel()
		s.blockCancel = nil
	}
}

func (s *service) SetCallbacks(cb ports.SchedulerCallbacks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = cb
}

func (s *service) ScheduleRefundAtHeight(target uint32, refund func()) error {
	if target <= 0 {
		return fmt.Errorf("invalid height: %d", target)
	}

	currentHeight, err := s.explorer.GetBlockHeight(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get current block height: %w", err)
	}
	if uint32(currentHeight) >= target {
		go refund()
		return nil
	}
	tsk := &heightTask{target: target, fn: refund}
	s.mu.Lock()
	s.tasks = append(s.tasks, tsk)
	s.mu.Unlock()
	return nil
}

func (s *service) ScheduleRefundAtTime(at time.Time, refund func()) error {
	if at.IsZero() {
		return fmt.Errorf("invalid schedule time")
	}

	delay := time.Until(at)
	if delay <= 0 {
		go refund()
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.scheduler.Every(delay).WaitForSchedule().LimitRunsTo(1).Do(func() {
		refund()
		s.mu.Lock()
		defer s.mu.Unlock()
	})
	if err != nil {
		return err
	}

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

	s.CancelNextSettlement()

	s.mu.Lock()
	defer s.mu.Unlock()

	if delay == 0 {
		settleFunc()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	job, err := s.scheduler.Every(delay).WaitForSchedule().LimitRunsTo(1).Do(func() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		settleFunc()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.scheduler.Remove(s.job)
		s.job = nil
	})
	if err != nil {
		cancel()
		return err
	}

	s.job = job
	s.stopJob = cancel

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

func (s *service) CancelNextSettlement() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.job == nil {
		return
	}

	s.stopJob()
	s.scheduler.Remove(s.job)
	s.job = nil
}
