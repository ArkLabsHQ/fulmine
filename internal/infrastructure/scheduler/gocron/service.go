package scheduler

import (
	"fmt"
	"time"

	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/go-co-op/gocron"
	"github.com/sirupsen/logrus"
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
	sooner := time.Now().Unix() + data.RoundLifetime

	for _, tx := range txs {
		if !tx.Pending {
			continue
		}
		expiresAt := tx.CreatedAt.Unix() + data.RoundLifetime
		if expiresAt < sooner {
			sooner = expiresAt
		}
	}

	// cancel previous job
	s.scheduler.Remove(s.job)

	delay := sooner - time.Now().Unix()
	if delay < 0 {
		return fmt.Errorf("cannot schedule task in the past")
	}

	// run it on the previous round
	if delay > data.RoundInterval {
		delay = delay - data.RoundInterval
	} else {
		delay = 0 // run immediatelly
	}
	logrus.Infof("setting scheduler for %d", sooner)

	// schedule job and store it in service
	job, err := s.scheduler.Every(int(delay)).Seconds().WaitForSchedule().LimitRunsTo(1).Do(task)
	s.job = job

	return err
}

func (s *service) WhenNextClaim() time.Time {
	return s.job.NextRun()
}
