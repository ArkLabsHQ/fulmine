package ports

import (
	"time"

	storetypes "github.com/ark-network/ark/pkg/client-sdk/store/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []storetypes.Transaction, data *storetypes.ConfigData, claimFunc func()) error
	WhenNextClaim() time.Time
}
