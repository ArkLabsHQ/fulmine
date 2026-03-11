package application

// ChainSwapEventType represents the type of chain swap lifecycle event.
type ChainSwapEventType string

const (
	ChainSwapEventCreated      ChainSwapEventType = "chain_swap_created"
	ChainSwapEventUserLocked   ChainSwapEventType = "chain_swap_user_locked"
	ChainSwapEventServerLocked ChainSwapEventType = "chain_swap_server_locked"
	ChainSwapEventClaimed      ChainSwapEventType = "chain_swap_claimed"
	ChainSwapEventRefunded     ChainSwapEventType = "chain_swap_refunded"
	ChainSwapEventFailed       ChainSwapEventType = "chain_swap_failed"
)

// ChainSwapRefundKind distinguishes cooperative from unilateral refunds.
type ChainSwapRefundKind string

const (
	ChainSwapRefundCooperative ChainSwapRefundKind = "cooperative"
	ChainSwapRefundUnilateral  ChainSwapRefundKind = "unilateral"
)

// ChainSwapEvent is emitted at each stage of the chain swap lifecycle.
// Consumers receive these via Service.GetChainSwapEvents().
// Fields not relevant to a given event type are zero-valued.
type ChainSwapEvent struct {
	Type           ChainSwapEventType
	Timestamp      int64
	SwapId         string              // Boltz swap ID
	Direction      string              // "ark_to_btc" or "btc_to_ark" (set on created)
	Amount         uint64              // (set on created)
	UserLockTxId   string              // (set on user_locked)
	ServerLockTxId string              // (set on server_locked)
	ClaimTxId      string              // (set on claimed)
	RefundTxId     string              // (set on refunded)
	RefundKind     ChainSwapRefundKind // (set on refunded)
	ErrorMessage   string              // (set on failed)
}
