package ports

import (
	"time"

	"github.com/ark-network/ark/pkg/client-sdk/store/domain"
)

type SchedulerService interface {
	Start()
	Stop()
	ScheduleNextClaim(txs []domain.Transaction, data *domain.ConfigData, claimFunc func()) error
	WhenNextClaim() time.Time
}
