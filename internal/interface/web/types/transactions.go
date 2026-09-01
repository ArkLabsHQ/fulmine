package types

import clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"

type PoolTxs struct {
	DateCreated int64              `json:"dateCreated"`
	Vtxos       []clientTypes.Vtxo `json:"vtxos"`
}

type Transaction struct {
	// Kind can be "swap", "transfer", "payment" or "chainswap"
	Kind string `json:"kind"`

	Id string `json:"id"`

	DateCreated int64 `json:"dateCreated"`

	// Exactly one of these will be non-nil:
	Transfer *Transfer `json:"transfer,omitempty"`
}

type Transfer struct {
	Amount     string `json:"amount"`
	CreatedAt  string `json:"createdAt"`
	Day        string `json:"day"`
	Explorable bool   `json:"explorable"`
	Hour       string `json:"hour"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Txid       string `json:"txid"`
	UnixDate   int64  `json:"unixdate"`
}
