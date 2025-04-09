package ports

import (
	"time"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextSettlement(at time.Time, settleFunc func()) error
	WhenNextSettlement() time.Time
}
