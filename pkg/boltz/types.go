package boltz

const (
	CurrencyBtc       Currency = "BTC"
	CurrencyArk       Currency = "ARK"
	CurrencyLiquid    Currency = "L-BTC"
	CurrencyRootstock Currency = "RBTC"
)

type Currency string

type TimeoutBlockHeights struct {
	RefundLocktime                  uint32 `json:"refund"`
	UnilateralClaim                 uint32 `json:"unilateralClaim"`
	UnilateralRefund                uint32 `json:"unilateralRefund"`
	UnilateralRefundWithoutReceiver uint32 `json:"unilateralRefundWithoutReceiver"`
}

type FetchBolt12InvoiceRequest struct {
	Offer  string `json:"offer"`
	Amount uint64 `json:"amount,omitempty"`
	Note   string `json:"note,omitempty"`
}

type FetchBolt12InvoiceResponse struct {
	Invoice string `json:"invoice"`

	Error string `json:"error"`
}

type CreateSwapRequest struct {
	From            Currency `json:"from"`
	To              Currency `json:"to"`
	RefundPublicKey string   `json:"refundPublicKey"`
	Invoice         string   `json:"invoice,omitempty"`
	PaymentTimeout  uint32   `json:"paymentTimeout,omitempty"`
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
	PreimageHash   string   `json:"preimageHash,omitempty"`
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

type GetSwapLimitsResponse struct {
	Ark struct {
		Btc struct {
			Limits struct {
				Maximal int `json:"maximal"`
				Minimal int `json:"minimal"`
			} `json:"limits"`
		} `json:"BTC"`
	} `json:"ARK"`
}

type RevealPreimageRequest struct {
	Id       string `json:"id"`
	Preimage string `json:"preimage"`
}

type RevealPreimageResponse struct {
	Id          string `json:"id"`
	Transaction string `json:"transaction"`

	Error string `json:"error"`
}

type RefundSwapRequest struct {
	Transaction string `json:"transaction"`
	Checkpoint  string `json:"checkpoint"`
}

type RefundSwapResponse struct {
	Transaction string `json:"transaction"`
	Checkpoint  string `json:"checkpoint"`
	Error       string `json:"error"`
}

type TransactionRef struct {
	ID   string `json:"id"`
	Vout int    `json:"vout"`
}

type Leaf struct {
	Version int    `json:"version"`
	Output  string `json:"output"`
}

type Tree struct {
	ClaimLeaf                       Leaf `json:"claimLeaf"`
	RefundLeaf                      Leaf `json:"refundLeaf"`
	RefundLeafWithoutReceiver       Leaf `json:"refundWithoutBoltzLeaf"`
	UnilateralClaimLeaf             Leaf `json:"unilateralClaimLeaf"`
	UnilateralRefundLeaf            Leaf `json:"unilateralRefundLeaf"`
	UnilateralRefundWithoutReceiver Leaf `json:"unilateralRefundWithoutBoltzLeaf"`
}

type SwapDetails struct {
	Tree               Tree           `json:"tree"`
	Amount             uint64         `json:"amount"`
	Transaction        TransactionRef `json:"transaction,omitempty"`
	LockupAddress      string         `json:"lockupAddress"`
	TimeoutBlockHeight uint32         `json:"timeoutBlockHeight"`
}

type FetchSwapHistoryResponse = []struct {
	Id            string       `json:"id"`
	Type          string       `json:"type"`
	Status        string       `json:"status"`
	From          Currency     `json:"from"`
	To            Currency     `json:"to"`
	CreateAt      uint32       `json:"createdAt"`
	PreimageHash  string       `json:"preimageHash"`
	ClaimDetails  *SwapDetails `json:"claimDetails,omitempty"`
	RefundDetails *SwapDetails `json:"refundDetails,omitempty"`
}
