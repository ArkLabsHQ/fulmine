package ports

import (
	"time"

	"github.com/ark-network/ark/pkg/client-sdk/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []types.Transaction, data *types.Config, claimFunc func()) error
	WhenNextClaim() time.Time
}
