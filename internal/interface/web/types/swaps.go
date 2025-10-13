package types

type Swap struct {
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Hour   string `json:"hour"`
	Id     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`

	ExpiresAt string `json:"expiry"`

	RefundLockTime *LockTime `json:"refundLockTime,omitempty"`

	// If Swap is Outbound, this is the Sent VHTLC
	VHTLCTransfer *Transfer `json:"vhtlc,omitempty"`
	// If Swap is Inbound, this is the Redeem Tx, else this is Txn of reclaim Failed Outbound Swap
	RedeemTransfer *Transfer `json:"redeemTransfer,omitempty"`
}
