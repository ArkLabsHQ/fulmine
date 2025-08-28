package types

type Payment struct {
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Hour   string `json:"hour"`
	Id     string `json:"id"`
	Kind   string `json:"kind"`   // "send" or "receive"
	Status string `json:"status"` // "pending", "refunding", "failed",  "completed"

	// This is the invoice expiration time
	ExpiresAt string `json:"expiresAt"`

	RefundLockTime *LockTime `json:"refundLockTime,omitempty"`

	PaymentTransfer *Transfer `json:"vhtlc,omitempty"`

	// If Payment Is Send and Failed
	ReclaimTransfer *Transfer `json:"redeemTransfer,omitempty"`
}

type LockTime struct {
	Timelock  string `json:"lockTime"`
	IsSeconds bool   `json:"isSecs"`
}
