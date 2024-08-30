package types

import "github.com/ark-network/ark/pkg/client-sdk/client"

type PoolTxs struct {
	DateCreated int64         `json:"dateCreated"`
	Vtxos       []client.Vtxo `json:"vtxos"`
}

type Transaction struct {
	Amount   int64  `json:"amount"`
	Date     string `json:"date"`
	Refresh  int64  `json:"refresh,omitempty"`
	Txid     string `json:"txid"`
	UnixDate int64  `json:"unixdate"`
}
