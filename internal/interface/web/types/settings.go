package types

type Settings struct {
	ApiRoot     string `json:"apiroot"`
	Currency    string `json:"currency"`
	EventServer string `json:"eventserver"`
	FullNode    string `json:"fullnode"`
	LnConnect   bool   `json:"lnconnected"`
	LnUrl       string `json:"lnurl"`
	Unit        string `json:"unit"`
}
