package application

// HtlcEventType represents the type of HTLC lifecycle event.
type HtlcEventType string

const (
	HtlcEventCreated    HtlcEventType = "htlc_created"
	HtlcEventFunded     HtlcEventType = "htlc_funded"
	HtlcEventSpent      HtlcEventType = "htlc_spent"
	HtlcEventRefundable HtlcEventType = "htlc_refundable"
)

// SpendType indicates whether a VHTLC was claimed or refunded.
type SpendType string

const (
	SpendTypeClaimed  SpendType = "claimed"
	SpendTypeRefunded SpendType = "refunded"
)

// HtlcEvent is emitted at each stage of the VHTLC lifecycle.
// Consumers receive these via Service.GetHtlcEvents().
type HtlcEvent struct {
	Type           HtlcEventType
	Timestamp      int64
	VhtlcId        string
	Address        string // VHTLC address (set for htlc_created)
	TxId           string // funding_txid for funded; redeem_txid for spent
	Amount         uint64
	SpendKind      SpendType // only for htlc_spent
	RefundLocktime uint64    // only for htlc_refundable
}
