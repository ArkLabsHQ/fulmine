package domain

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
)

type SwapStatus int

const (
	SwapPending SwapStatus = iota
	SwapFailed
	SwapSuccess
)

type SwapType int

const (
	SwapRegular SwapType = iota
	SwapPayment
)

type Swap struct {
	Id          string
	Amount      uint64
	Timestamp   int64
	To          boltz.Currency
	From        boltz.Currency
	Status      SwapStatus
	Type        SwapType
	Invoice     string
	VhtlcOpts   vhtlc.Opts
	FundingTxId string // the txid of the virtual tx that funded the vhtlc
	RedeemTxId  string // the txid of the virtual tx that redeemed the funds, by either "claiming" or "refunding"
}

// SwapRepository stores the Swap initiated by the wallet
type SwapRepository interface {
	GetAll(ctx context.Context) ([]Swap, error)
	Get(ctx context.Context, swapId string) (*Swap, error)
	Add(ctx context.Context, swap Swap) error
	Update(ctx context.Context, swap Swap) error
	Close()
}
