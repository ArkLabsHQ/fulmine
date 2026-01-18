package types

type Transfer struct {
	Amount     string `json:"amount"`
	ArkTxid    string `json:"arkTxid"`
	CreatedAt  string `json:"createdAt"`
	Day        string `json:"day"`
	Explorable bool   `json:"explorable"`
	Hour       string `json:"hour"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Txid       string `json:"txid"`
	UnixDate   int64  `json:"unixdate"`
}
