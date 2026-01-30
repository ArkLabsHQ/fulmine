package boltz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Api struct {
	URL    string
	WSURL  string
	Client http.Client
}

func (boltz *Api) CreateReverseSwap(request CreateReverseSwapRequest) (*CreateReverseSwapResponse, error) {
	limits, err := sendGetRequest[GetSwapLimitsResponse](boltz, "/swap/submarine")
	if err != nil {
		return nil, err
	}

	if limits.Ark.Btc.Limits.Minimal > int(request.InvoiceAmount) || limits.Ark.Btc.Limits.Maximal < int(request.InvoiceAmount) {
		return nil, fmt.Errorf("out of limits: invoice amount %d must be between %d and %d", request.InvoiceAmount,
			limits.Ark.Btc.Limits.Minimal, limits.Ark.Btc.Limits.Maximal)
	}

	resp, err := sendPostRequest[CreateReverseSwapResponse](boltz, "/swap/reverse", request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) FetchBolt12Invoice(request FetchBolt12InvoiceRequest) (*FetchBolt12InvoiceResponse, error) {
	resp, err := sendPostRequest[FetchBolt12InvoiceResponse](boltz, "/lightning/BTC/bolt12/fetch", request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) CreateSwap(request CreateSwapRequest) (*CreateSwapResponse, error) {
	resp, err := sendPostRequest[CreateSwapResponse](boltz, "/swap/submarine", request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) RefundChainSwap(swapId string, request RefundSwapRequest) (*RefundSwapResponse, error) {
	url := fmt.Sprintf("/swap/chain/%s/refund/ark", swapId)
	resp, err := sendPostRequest[RefundSwapResponse](boltz, url, request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) RefundSubmarine(swapId string, request RefundSwapRequest) (*RefundSwapResponse, error) {
	url := fmt.Sprintf("/swap/submarine/%s/refund/ark", swapId)
	resp, err := sendPostRequest[RefundSwapResponse](boltz, url, request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) RevealPreimage(swapId string, preimage string) (*RevealPreimageResponse, error) {
	url := fmt.Sprintf("/swap/reverse/%s/reveal/ark", swapId)
	request := RevealPreimageRequest{Preimage: preimage}
	resp, err := sendPostRequest[RevealPreimageResponse](boltz, url, request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) GetSwapHistory(pubkey string) ([]Swap, error) {
	url := "/swap/restore"

	request := struct {
		PublicKey string `json:"publicKey"`
	}{
		PublicKey: pubkey,
	}
	resp, err := sendPostRequest[[]Swap](boltz, url, request)
	if err != nil {
		return nil, err
	}
	return *resp, nil
}

// Chain Swap API Methods

func (boltz *Api) CreateChainSwap(request CreateChainSwapRequest) (*CreateChainSwapResponse, error) {
	resp, err := sendPostRequest[CreateChainSwapResponse](boltz, "/swap/chain", request)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp, nil
}

func (boltz *Api) GetChainSwapClaimDetails(swapId string) (*ChainSwapClaimDetailsResponse, error) {
	url := fmt.Sprintf("/swap/chain/%s/claim", swapId)
	return sendGetRequest[ChainSwapClaimDetailsResponse](boltz, url)
}

func (boltz *Api) SubmitChainSwapClaim(swapId string, request ChainSwapClaimRequest) (*PartialSignatureResponse, error) {
	url := fmt.Sprintf("/swap/chain/%s/claim", swapId)
	resp, err := sendPostRequest[PartialSignatureResponse](boltz, url, request)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (boltz *Api) GetChainSwapQuote(swapId string) (*QuoteResponse, error) {
	url := fmt.Sprintf("/swap/chain/%s/quote", swapId)
	return sendGetRequest[QuoteResponse](boltz, url)
}

func (boltz *Api) AcceptChainSwapQuote(swapId string, quote QuoteResponse) error {
	url := fmt.Sprintf("/swap/chain/%s/quote", swapId)
	_, err := sendPostRequest[QuoteResponse](boltz, url, quote)
	return err
}

func (boltz *Api) GetChainSwapTransactions(swapId string) (*ChainSwapTransactionsResponse, error) {
	url := fmt.Sprintf("/swap/chain/%s/transactions", swapId)
	return sendGetRequest[ChainSwapTransactionsResponse](boltz, url)
}

const defaultHTTPTimeout = 15 * time.Second

func withTimeoutCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultHTTPTimeout)
}

func sendGetRequest[T any](boltz *Api, endpoint string) (*T, error) {
	ctx, cancel := withTimeoutCtx()
	defer cancel()

	url := boltz.URL + "/v2" + endpoint
	return callApi[T](ctx, &boltz.Client, http.MethodGet, url, nil)
}

func sendPostRequest[T any](boltz *Api, endpoint string, requestBody any) (*T, error) {
	ctx, cancel := withTimeoutCtx()
	defer cancel()

	url := boltz.URL + "/v2" + endpoint
	return callApi[T](ctx, &boltz.Client, http.MethodPost, url, requestBody)
}

func callApi[T any](ctx context.Context, c *http.Client, method, url string, reqBody any) (*T, error) {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("new %s %s: %w", method, url, err)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// If not 2xx, return a real error (include body for debugging)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Try to extract a JSON "error" message if present, otherwise include raw (truncated)
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 2000 {
			msg = msg[:2000] + "...(truncated)"
		}
		return nil, &HTTPError{
			Method:     method,
			URL:        url,
			StatusCode: res.StatusCode,
			Body:       msg,
		}
	}

	// Handle empty body for 204 etc.
	if len(bytes.TrimSpace(raw)) == 0 {
		var zero T
		return &zero, nil
	}

	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		// Helpful when server returns HTML or plain text
		snip := strings.TrimSpace(string(raw))
		if len(snip) > 300 {
			snip = snip[:300] + "...(truncated)"
		}
		return nil, fmt.Errorf("unmarshal JSON: %w (body: %q)", err, snip)
	}

	return &out, nil
}

type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}
