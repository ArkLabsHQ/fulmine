package types

import "github.com/ark-network/ark/pkg/client-sdk/client"

type PoolTxs struct {
	DateCreated int64         `json:"dateCreated"`
	Vtxos       []client.Vtxo `json:"vtxos"`
}

type Transaction struct {
	Amount   string `json:"amount"`
	Date     string `json:"date"`
	Day      string `json:"day"`
	Hour     string `json:"hour"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Txid     string `json:"txid"`
	UnixDate int64  `json:"unixdate"`
}
