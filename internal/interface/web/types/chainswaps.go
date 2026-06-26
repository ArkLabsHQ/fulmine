package types

// ChainSwap is the web view of a domain.ChainSwap (an on-chain BTC<->ARK swap,
// distinct from the submarine/Lightning Swap). It surfaces in the unified tx
// history and on its own detail page.
type ChainSwap struct {
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Hour   string `json:"hour"`
	Id     string `json:"id"`
	// Kind is the direction: "ark_to_btc" or "btc_to_ark".
	Kind string `json:"kind"`
	// Status is one of "pending" / "refunding" / "success" / "failure".
	Status string `json:"status"`
	// UserBtcLockupAddress is the BTC lockup address (shown on BTC->ARK swaps).
	UserBtcLockupAddress string `json:"userBtcLockupAddress,omitempty"`
}
