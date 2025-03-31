package scheduler

import (
	"fmt"
	"math"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ark-network/ark/pkg/client-sdk/client"
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

// Sets a ClaimPending() to run in the best market hour
// Besides claiming, ClaimPending() also calls this function
func (s *service) ScheduleNextClaim(spendableVtxos []client.Vtxo, cfg *types.Config, claimFunc func()) error {
	if len(spendableVtxos) == 0 {
		return nil
	}

	var at *time.Time

	for _, vtxo := range spendableVtxos {
		if at == nil || vtxo.ExpiresAt.Before(*at) {
			at = &vtxo.ExpiresAt
		}
	}

	bestTime := bestMarketHour(at.Unix(), cfg.MarketHourStartTime, cfg.MarketHourPeriod)
	delay := bestTime - time.Now().Unix()
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

func (s *service) WhenNextClaim() (*time.Time, error) {
	if s.job == nil {
		return nil, fmt.Errorf("no job scheduled")
	}

	nextRun := s.job.NextRun()
	return &nextRun, nil
}

func bestMarketHour(expiresAt, nextMarketHour, marketHourPeriod int64) int64 {
	if expiresAt < nextMarketHour {
		return expiresAt
	}

	cycles := int64(math.Floor(float64(expiresAt-nextMarketHour) / float64(marketHourPeriod)))

	return nextMarketHour + (cycles * marketHourPeriod)
}
