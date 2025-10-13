package boltz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

const defaultHTTPTimeout = 15 * time.Second

func withTimeoutCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultHTTPTimeout)
}

func sendGetRequest[T any](boltz *Api, endpoint string) (*T, error) {
	ctx, cancel := withTimeoutCtx()
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, boltz.URL+"/v2"+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("new GET %s: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := boltz.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", endpoint, err)
	}

	return unmarshalJson[T](res.Body)
}

func sendPostRequest[T any](boltz *Api, endpoint string, requestBody any) (*T, error) {
	rawBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := withTimeoutCtx()
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, boltz.URL+"/v2"+endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("new POST %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := boltz.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", endpoint, err)

	}

	return unmarshalJson[T](res.Body)
}

func unmarshalJson[T any](body io.ReadCloser) (*T, error) {
	rawBody, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var res T
	if err := json.Unmarshal(rawBody, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
