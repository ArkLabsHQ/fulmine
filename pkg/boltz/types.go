package boltz

const (
	CurrencyBtc       Currency = "BTC"
	CurrencyArk       Currency = "ARK"
	CurrencyLiquid    Currency = "L-BTC"
	CurrencyRootstock Currency = "RBTC"
)

type Currency string

type TimeoutBlockHeights struct {
	UnilateralClaim                 uint32 `json:"unilateralClaim"`
	UnilateralRefund                uint32 `json:"unilateralRefund"`
	UnilateralRefundWithoutReceiver uint32 `json:"unilateralRefundWithoutReceiver"`
}

type CreateSwapRequest struct {
	From            Currency `json:"from"`
	To              Currency `json:"to"`
	RefundPublicKey string   `json:"refundPublicKey"`
	Invoice         string   `json:"invoice,omitempty"`
}

type CreateSwapResponse struct {
	Id                  string              `json:"id"`
	Address             string              `json:"address"`
	AcceptZeroConf      bool                `json:"acceptZeroConf"`
	ExpectedAmount      uint64              `json:"expectedAmount"`
	ClaimPublicKey      string              `json:"claimPublicKey"`
	TimeoutBlockHeights TimeoutBlockHeights `json:"timeoutBlockHeights"`

	Error string `json:"error"`
}

type CreateReverseSwapRequest struct {
	From           Currency `json:"from"`
	To             Currency `json:"to"`
	ClaimPublicKey string   `json:"claimPublicKey"`
	InvoiceAmount  uint64   `json:"invoiceAmount,omitempty"`
	OnchainAmount  uint64   `json:"onchainAmount,omitempty"`
}

type CreateReverseSwapResponse struct {
	Id                  string              `json:"id"`
	LockupAddress       string              `json:"lockupAddress"`
	RefundPublicKey     string              `json:"refundPublicKey"`
	TimeoutBlockHeights TimeoutBlockHeights `json:"timeoutBlockHeights"`
	Invoice             string              `json:"invoice"`
	InvoiceAmount       uint64              `json:"invoiceAmount,omitempty"`
	OnchainAmount       uint64              `json:"onchainAmount"`

	Error string `json:"error"`
}

type RefundSwapRequest struct {
	Transaction string `json:"transaction"`
}

type RefundSwapResponse struct {
	Transaction string `json:"transaction"`
	Error       string `json:"error"`
}
