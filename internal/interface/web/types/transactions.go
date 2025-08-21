package types

import "github.com/arkade-os/go-sdk/types"

type PoolTxs struct {
	DateCreated int64        `json:"dateCreated"`
	Vtxos       []types.Vtxo `json:"vtxos"`
}

type Transaction struct {
	// Kind can be "swap" or "transfer" or "payment"
	Kind string `json:"kind"`

	Id string `json:"id"`

	DateCreated int64 `json:"dateCreated"`

	// Exactly one of these will be non-nil:
	Swap     *Swap     `json:"swap,omitempty"`
	Transfer *Transfer `json:"transfer,omitempty"`
	Payment  *Payment  `json:"payment,omitempty"`
}
