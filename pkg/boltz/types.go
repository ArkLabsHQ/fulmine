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

type Swap struct {
	Id            string       `json:"id"`
	Type          string       `json:"type"`
	Status        string       `json:"status"`
	From          Currency     `json:"from"`
	To            Currency     `json:"to"`
	CreatedAt     uint32       `json:"createdAt"`
	PreimageHash  string       `json:"preimageHash"`
	ClaimDetails  *SwapDetails `json:"claimDetails,omitempty"`
	RefundDetails *SwapDetails `json:"refundDetails,omitempty"`
}

type CreateChainSwapRequest struct {
	From             Currency `json:"from"`
	To               Currency `json:"to"`
	PreimageHash     string   `json:"preimageHash"`
	ClaimPublicKey   string   `json:"claimPublicKey"`
	RefundPublicKey  string   `json:"refundPublicKey"`
	UserLockAmount   uint64   `json:"userLockAmount,omitempty"`
	ServerLockAmount uint64   `json:"serverLockAmount,omitempty"`
	PairHash         string   `json:"pairHash,omitempty"`
	ReferralId       string   `json:"referralId,omitempty"`
}

type ChainSwapLockupDetails struct {
	LockupAddress      string   `json:"lockupAddress"`
	Amount             uint64   `json:"amount"`
	ClaimPublicKey     string   `json:"claimPublicKey,omitempty"`
	RefundPublicKey    string   `json:"refundPublicKey,omitempty"`
	TimeoutBlockHeight uint32   `json:"timeoutBlockHeight"`
	SwapTree           SwapTree `json:"swapTree"`
}

type SwapTree struct {
	ClaimLeaf  SwapTreeLeaf `json:"claimLeaf"`
	RefundLeaf SwapTreeLeaf `json:"refundLeaf"`
}

type SwapTreeLeaf struct {
	Version uint8  `json:"version"`
	Output  string `json:"output"`
}

type ChainSwapTimeouts struct {
	Refund                          int `json:"refund"`
	UnilateralClaim                 int `json:"unilateralClaim"`
	UnilateralRefund                int `json:"unilateralRefund"`
	UnilateralRefundWithoutReceiver int `json:"unilateralRefundWithoutReceiver"`
}

type CreateChainSwapResponse struct {
	Id           string      `json:"id"`
	ClaimDetails SwapLeg     `json:"claimDetails"`
	LockupDetails SwapLeg    `json:"lockupDetails"`
	Error        string      `json:"error,omitempty"`
}

func (c CreateChainSwapResponse) GetSwapTree(isArkToBtc bool) SwapTree{
	if isArkToBtc {
		return SwapTree{
			ClaimLeaf: SwapTreeLeaf{
				Version: c.ClaimDetails.SwapTree.ClaimLeaf.Version,
				Output:  c.ClaimDetails.SwapTree.ClaimLeaf.Output,
			},
			RefundLeaf: SwapTreeLeaf{
				Version: c.ClaimDetails.SwapTree.RefundLeaf.Version,
				Output:  c.ClaimDetails.SwapTree.RefundLeaf.Output,
			},
		}
	}

	return SwapTree{
		ClaimLeaf: SwapTreeLeaf{
			Version: c.LockupDetails.SwapTree.ClaimLeaf.Version,
			Output:  c.LockupDetails.SwapTree.ClaimLeaf.Output,
		},
		RefundLeaf: SwapTreeLeaf{
			Version: c.LockupDetails.SwapTree.RefundLeaf.Version,
			Output:  c.LockupDetails.SwapTree.RefundLeaf.Output,
		},
	}
}

// SwapLeg describes ONE side (one chain) of the swap.
// Some fields exist only for BTC (swapTree, bip21) or only for ARK (timeouts).
type SwapLeg struct {
	ServerPublicKey    string       `json:"serverPublicKey"`
	Amount             int          `json:"amount"`
	LockupAddress      string       `json:"lockupAddress"`
	TimeoutBlockHeight int          `json:"timeoutBlockHeight"`

	// BTC-specific (present on the BTC leg; may appear on either claimDetails or lockupDetails)
	SwapTree *SwapTree `json:"swapTree,omitempty"`
	// ARK-specific (present on the ARK leg; may appear on either claimDetails or lockupDetails)
	Timeouts *ArkTimeouts `json:"timeouts,omitempty"`
}

type TapLeaf struct {
	Version int    `json:"version"`
	Output  string `json:"output"`
}

type ArkTimeouts struct {
	Refund                          int `json:"refund"`
	UnilateralClaim                 int `json:"unilateralClaim"`
	UnilateralRefund                int `json:"unilateralRefund"`
	UnilateralRefundWithoutReceiver int `json:"unilateralRefundWithoutReceiver"`
}

type ChainSwapClaimDetailsResponse struct {
	PubNonce        string         `json:"pubNonce"`
	PublicKey       string         `json:"publicKey"`
	TheirPublicKey  string         `json:"theirPublicKey"`
	TransactionHash string         `json:"transactionHash"`
	SwapTree        string         `json:"swapTree"` // base64 serialized
	Transaction     TransactionRef `json:"transaction"`
}

type ChainSwapClaimRequest struct {
	Preimage         string              `json:"preimage"`
	ToSign           ToSign              `json:"toSign"`
	PubNonce         string              `json:"pubNonce"`
	PartialSignature string              `json:"partialSignature"`
	Transaction      string              `json:"transaction"`
	Index            int                 `json:"index"`
	Signature        CrossSignSignature `json:"signature,omitempty"`
}

type ToSign struct {
	Nonce   string `json:"pubNonce"`
	ClaimTx string `json:"transaction"`
	Index   int    `json:"index"`
}

type CrossSignSignature struct {
	PubNonce         string `json:"pubNonce"`
	PartialSignature string `json:"partialSignature"`
}

type PartialSignatureResponse struct {
	PubNonce         string `json:"pubNonce"`
	PartialSignature string `json:"partialSignature"`
}

type QuoteResponse struct {
	Amount             uint64 `json:"amount"`
	OnchainAmount      uint64 `json:"onchainAmount"`
	TimeoutBlockHeight uint32 `json:"timeoutBlockHeight"`
}

type ChainSwapTransactionsResponse struct {
	UserLock   *ChainSwapTransaction `json:"userLock,omitempty"`
	ServerLock *ChainSwapTransaction `json:"serverLock,omitempty"`
}

type ChainSwapTransaction struct {
	Id     string `json:"id"`
	Hex    string `json:"hex"`
	Status string `json:"status"`
}
