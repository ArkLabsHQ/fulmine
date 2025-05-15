package domain

import (
	"context"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
)

type Swap struct {
	Id        string
	Amount    uint64
	Date      time.Time
	To        boltz.Currency
	From      boltz.Currency
	IsPending bool
	Invoice   string
	VHltcId   string
}

// SwapRepository stores the Swap initiated by the wallet
type SwapRepository interface {
	GetAll(ctx context.Context) ([]Swap, error)
	Get(ctx context.Context, invoice string) (*Swap, error)
	Add(ctx context.Context, swap Swap) error
	Delete(ctx context.Context, invoice string) error
	Close()
}
