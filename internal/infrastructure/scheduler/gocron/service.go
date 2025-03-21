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
	job := gocron.Job{}
	return &service{svc, &job}
}

func (s *service) Start() {
	s.scheduler.StartAsync()
}

func (s *service) Stop() {
	s.scheduler.Stop()
}

// Sets a ClaimPending() to run in the best market hour
// Besides claiming, ClaimPending() also calls this function
func (s *service) ScheduleNextClaim(txs []types.Transaction, cfg *types.Config, claimFunc func()) error {
	now := time.Now().Unix()
	at := now + int64(cfg.VtxoTreeExpiry.Value)

	for _, tx := range txs {
		if tx.Settled {
			continue
		}
		expiresAt := tx.CreatedAt.Unix() + int64(cfg.VtxoTreeExpiry.Value)
		if expiresAt < at {
			at = expiresAt
		}
	}

	bestTime := bestMarketHour(at, cfg.RoundInterval)

	delay := bestTime - now
	if delay < 0 {
		return fmt.Errorf("cannot schedule task in the past")
	}

	s.scheduler.Remove(s.job)

	job, err := s.scheduler.Every(int(delay)).Seconds().WaitForSchedule().LimitRunsTo(1).Do(claimFunc)
	if err != nil {
		return err
	}

	s.job = job

	return err
}

func (s *service) WhenNextClaim() time.Time {
	return s.job.NextRun()
}

// TODO: get market hour info from config data
func bestMarketHour(at int64, roundInterval int64) int64 {
	firstMarketHour := int64(1231006505) // block 0 timestamp
	marketHourInterval := int64(86400)   // 24 hours
	cycles := (at - firstMarketHour) / marketHourInterval
	best := firstMarketHour + cycles*marketHourInterval
	if at-best <= roundInterval {
		return best - marketHourInterval
	}
	return best
}
