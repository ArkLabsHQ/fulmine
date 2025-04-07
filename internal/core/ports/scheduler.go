package ports

import (
	"time"

	"github.com/ark-network/ark/pkg/client-sdk/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextSettlement(expiryAt time.Time, cfg *types.Config, settleFunc func()) error
	WhenNextSettlement() time.Time
}
