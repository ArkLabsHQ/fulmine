package ports

import (
	"time"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextSettlement(at time.Time, settleFunc func()) error
	ScheduleRefundAtTime(at time.Time, refundFunc func()) error
	ScheduleRefundAtHeight(target uint32, refund func()) error
	WhenNextSettlement() time.Time
}
