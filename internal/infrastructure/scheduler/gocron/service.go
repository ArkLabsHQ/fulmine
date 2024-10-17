package scheduler

import (
	"fmt"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	sdktypes "github.com/ark-network/ark/pkg/client-sdk/types"
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
func (s *service) ScheduleNextClaim(txs []sdktypes.Transaction, data *sdktypes.Config, claimFunc func()) error {
	now := time.Now().Unix()
	at := now + data.RoundLifetime

	for _, tx := range txs {
		if !tx.IsPending {
			continue
		}
		expiresAt := tx.CreatedAt.Unix() + data.RoundLifetime
		if expiresAt < at {
			at = expiresAt
		}
	}

	bestTime := bestMarketHour(at, data)

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
func bestMarketHour(at int64, data *sdktypes.Config) int64 {
	firstMarketHour := int64(1231006505) // block 0 timestamp
	marketHourInterval := int64(86400)   // 24 hours
	cycles := (at - firstMarketHour) / marketHourInterval
	best := firstMarketHour + cycles*marketHourInterval
	if at-best <= data.RoundInterval {
		return best - marketHourInterval
	}
	return best
}
