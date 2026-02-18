package ports

import (
	"time"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextSettlement(at time.Time, settleFunc func()) error
	ScheduleTaskAtTime(at time.Time, taskFunc func()) error
	ScheduleTaskAtHeight(target uint32, taskFunc func()) error
	CancelNextSettlement()
	WhenNextSettlement() time.Time
}
