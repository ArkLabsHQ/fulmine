package types

import "github.com/arkade-os/go-sdk/types"

type PoolTxs struct {
	DateCreated int64        `json:"dateCreated"`
	Vtxos       []types.Vtxo `json:"vtxos"`
}

type Transaction struct {
	// Kind can be "swap" or "transfer"
	Kind string `json:"kind"`

	Id string `json:"id"`

	DateCreated int64 `json:"dateCreated"`

	// Exactly one of these will be non-nil:
	Swap     *Swap     `json:"swap,omitempty"`
	Transfer *Transfer `json:"transfer,omitempty"`

	// If Swap is Outbound, this is the Sent VHTLC
	VHTLCTransfer *Transfer `json:"vhtlc,omitempty"`
	// If Swap is Inbound, this is the Redeem Tx, else this is Txn of reclaim Failed Outbound Swap
	RedeemTransfer *Transfer `json:"redeemTransfer,omitempty"`
}
