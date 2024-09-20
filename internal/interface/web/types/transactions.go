package types

import "github.com/ark-network/ark/pkg/client-sdk/client"

type PoolTxs struct {
	DateCreated int64         `json:"dateCreated"`
	Vtxos       []client.Vtxo `json:"vtxos"`
}

type Transaction struct {
	Amount     string `json:"amount"`
	CreatedAt  string `json:"createdAt"`
	Day        string `json:"day"`
	ExpiresAt  string `json:"expiresAt"`
	Explorable bool   `json:"explorable"`
	Hour       string `json:"hour"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Txid       string `json:"txid"`
	UnixDate   int64  `json:"unixdate"`
}
