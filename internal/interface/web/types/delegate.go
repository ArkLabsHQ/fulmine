package types

type DelegateTask struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Fee           string `json:"fee"`
	ScheduledAt   string `json:"scheduledAt"`
	ScheduledDate string `json:"scheduledDate"`
	ScheduledHour string `json:"scheduledHour"`
	FailReason    string `json:"failReason,omitempty"`
	CommitmentTxid string `json:"commitmentTxid,omitempty"`
}
