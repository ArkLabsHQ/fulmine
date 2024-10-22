package ports

import (
	"time"

	sdktypes "github.com/ark-network/ark/pkg/client-sdk/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []sdktypes.Transaction, data *sdktypes.StoreData, claimFunc func()) error
	WhenNextClaim() time.Time
}
