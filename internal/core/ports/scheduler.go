package ports

import (
	"time"

	"github.com/ark-network/ark/pkg/client-sdk/client"
	"github.com/ark-network/ark/pkg/client-sdk/types"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(spendableVtxos []client.Vtxo, data *types.Config, claimFunc func()) error
	WhenNextClaim() (*time.Time, error)
}
