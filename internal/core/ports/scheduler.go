package ports

import (
	"time"
)

type SchedulerCallbacks struct {
	OnHeartbeat func()
	OnError     func(error)
}

type SchedulerService interface {
	Start()
	Stop()
	SetCallbacks(SchedulerCallbacks)
	ScheduleNextSettlement(at time.Time, settleFunc func()) error
	ScheduleRefundAtTime(at time.Time, refundFunc func()) error
	ScheduleRefundAtHeight(target uint32, refund func()) error
	CancelNextSettlement()
	WhenNextSettlement() time.Time
}
