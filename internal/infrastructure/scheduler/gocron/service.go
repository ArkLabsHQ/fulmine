package scheduler

import (
	"fmt"
	"time"

	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/go-co-op/gocron"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []arksdk.Transaction, data *store.StoreData, task func()) error
	WhenNextClaim() time.Time
}

type service struct {
	scheduler *gocron.Scheduler
	job       *gocron.Job
}

func NewScheduler() SchedulerService {
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

func (s *service) ScheduleNextClaim(txs []arksdk.Transaction, data *store.StoreData, task func()) error {
	now := time.Now().Unix()
	at := now + data.RoundLifetime

	for _, tx := range txs {
		if !tx.Pending {
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

	job, err := s.scheduler.Every(int(delay)).Seconds().WaitForSchedule().LimitRunsTo(1).Do(task)
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
func bestMarketHour(at int64, data *store.StoreData) int64 {
	firstMarketHour := int64(1231006505) // block 0 timestamp
	marketHourInterval := int64(86400)   // 24 hours
	cycles := (at - firstMarketHour) / marketHourInterval
	best := firstMarketHour + cycles*marketHourInterval
	if at-best <= data.RoundInterval {
		return best - marketHourInterval
	}
	return best
}
