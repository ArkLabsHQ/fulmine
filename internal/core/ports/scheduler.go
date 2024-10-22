package ports

import (
	"time"

	"github.com/ark-network/ark/pkg/client-sdk/store/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []types.Transaction, data *types.StoreData, claimFunc func()) error
	WhenNextClaim() time.Time
}
