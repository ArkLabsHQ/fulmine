package ports

import (
	"time"

	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/store"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []arksdk.Transaction, data *store.StoreData, claimFunc func()) error
	WhenNextClaim() time.Time
}
