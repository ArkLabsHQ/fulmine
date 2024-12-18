package cln

type GetInfoResponse struct {
	Id                  string        `json:"id"`
	Alias               string        `json:"alias"`
	Color               string        `json:"color"`
	NumPeers            int           `json:"num_peers"`
	NumPendingChannels  int           `json:"num_pending_channels"`
	NumActiveChannels   int           `json:"num_active_channels"`
	NumInactiveChannels int           `json:"num_inactive_channels"`
	Address             []interface{} `json:"address"`
	Binding             []struct {
		Type    string `json:"type"`
		Address string `json:"address"`
		Port    int    `json:"port"`
	} `json:"binding"`
	Version           string `json:"version"`
	Blockheight       int    `json:"blockheight"`
	Network           string `json:"network"`
	FeesCollectedMsat int    `json:"fees_collected_msat"`
	LightningDir      string `json:"lightning-dir"`
	OurFeatures       struct {
		Init    string `json:"init"`
		Node    string `json:"node"`
		Channel string `json:"channel"`
		Invoice string `json:"invoice"`
	} `json:"our_features"`
}

type CreateInvoiceResponse struct {
	PaymentHash     string `json:"payment_hash"`
	ExpiresAt       int    `json:"expires_at"`
	Bolt11          string `json:"bolt11"`
	PaymentSecret   string `json:"payment_secret"`
	CreatedIndex    int    `json:"created_index"`
	WarningCapacity string `json:"warning_capacity"`
}

type PayInvoiceResponse struct {
	Destination     string  `json:"destination"`
	PaymentHash     string  `json:"payment_hash"`
	CreatedAt       float64 `json:"created_at"`
	Parts           int     `json:"parts"`
	AmountMsat      int     `json:"amount_msat"`
	AmountSentMsat  int     `json:"amount_sent_msat"`
	PaymentPreimage string  `json:"payment_preimage"`
	Status          string  `json:"status"`
}
