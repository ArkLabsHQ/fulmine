package swap

import "github.com/ArkLabsHQ/fulmine/pkg/boltz"

// BoltzClient is the subset of the Boltz API the swap handlers depend on. It is
// extracted as an interface so the handlers' failure paths — which a cooperative
// live Boltz never exercises end-to-end — can be driven against a fake
// counterparty in unit tests. *boltz.Api satisfies it, so production wiring is
// unchanged.
type BoltzClient interface {
	CreateSwap(request boltz.CreateSwapRequest) (*boltz.CreateSwapResponse, error)
	CreateReverseSwap(request boltz.CreateReverseSwapRequest) (*boltz.CreateReverseSwapResponse, error)
	CreateChainSwap(request boltz.CreateChainSwapRequest) (*boltz.CreateChainSwapResponse, error)
	FetchBolt12Invoice(request boltz.FetchBolt12InvoiceRequest) (*boltz.FetchBolt12InvoiceResponse, error)
	GetChainSwapQuote(swapId string) (*boltz.QuoteResponse, error)
	AcceptChainSwapQuote(swapId string, quote boltz.QuoteResponse) error
	GetChainSwapClaimDetails(swapId string) (*boltz.ChainSwapClaimDetailsResponse, error)
	SubmitChainSwapClaim(swapId string, request boltz.ChainSwapClaimRequest) (*boltz.PartialSignatureResponse, error)
	RefundChainSwap(swapId string, request boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error)
	RefundSubmarine(swapId string, request boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error)
	NewWebsocket() *boltz.Websocket
}

// compile-time guarantee that the real client satisfies the interface.
var _ BoltzClient = (*boltz.Api)(nil)
