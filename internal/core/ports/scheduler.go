package ports

import (
	"time"

	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/store"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []arksdk.Transaction, data *store.StoreData, task func()) error
	WhenNextClaim() time.Time
}
