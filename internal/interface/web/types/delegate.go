package types

type DelegateTaskIntent struct {
	Txid    string   `json:"txid"`
	Message string   `json:"message"`
	Proof   string   `json:"proof"`
	Inputs  []string `json:"inputs"` // Format: "txid:vout"
}

type DelegateTaskForfeit struct {
	Outpoint string `json:"outpoint"` // Format: "txid:vout"
	Txid     string `json:"txid"`
}

type DelegateTask struct {
	ID                 string                `json:"id"`
	Status             string                `json:"status"`
	Fee                string                `json:"fee"`
	ScheduledAt        string                `json:"scheduledAt"`
	ScheduledAtUnix    int64                 `json:"scheduledAtUnix"` // Unix timestamp for client-side formatting
	ScheduledDate      string                `json:"scheduledDate"`     // Deprecated: use scheduledAtUnix
	ScheduledHour      string                `json:"scheduledHour"`     // Deprecated: use scheduledAtUnix
	FailReason         string                `json:"failReason,omitempty"`
	CommitmentTxid     string                `json:"commitmentTxid,omitempty"`
	DelegatorPublicKey string                `json:"delegatorPublicKey,omitempty"`
	Intent             *DelegateTaskIntent   `json:"intent,omitempty"`
	Forfeits           []DelegateTaskForfeit `json:"forfeits,omitempty"`
}
