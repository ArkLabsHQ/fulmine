package types

type Settings struct {
	ApiRoot     string `json:"apiroot"`
	ServerUrl   string `json:"serverurl"`
	Currency    string `json:"currency"`
	EventServer string `json:"eventserver"`
	FullNode    string `json:"fullnode"`
	LnUrl       string `json:"lnurl"`
	Unit        string `json:"unit"`
}
