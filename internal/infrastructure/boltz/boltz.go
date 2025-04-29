package boltz

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Api struct {
	URL    string
	Client http.Client
}

func (boltz *Api) CreateReverseSwap(request CreateReverseSwapRequest) (*CreateReverseSwapResponse, error) {
	var response CreateReverseSwapResponse
	err := boltz.sendPostRequest("/swap/reverse", request, &response)

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	return &response, err
}

func (boltz *Api) CreateSwap(request CreateSwapRequest) (*CreateSwapResponse, error) {
	var response CreateSwapResponse
	err := boltz.sendPostRequest("/swap/submarine", request, &response)

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	return &response, err
}

func (boltz *Api) RefundSubmarine(swapId string, request RefundSwapRequest) (*RefundSwapResponse, error) {
	var response RefundSwapResponse
	err := boltz.sendPostRequest(fmt.Sprintf("/swap/submarine/%s/refund/ark", swapId), request, &response)

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	return &response, err
}

func (boltz *Api) sendPostRequest(endpoint string, requestBody interface{}, response interface{}) error {
	rawBody, err := json.Marshal(requestBody)

	if err != nil {
		return err
	}

	res, err := boltz.Client.Post(boltz.URL+"/v2"+endpoint, "application/json", bytes.NewBuffer(rawBody))

	if err != nil {
		return err
	}

	if err := unmarshalJson(res.Body, &response); err != nil {
		return fmt.Errorf("could not parse boltz response with status %d: %v", res.StatusCode, err)
	}
	return nil
}

func unmarshalJson(body io.ReadCloser, response interface{}) error {
	rawBody, err := io.ReadAll(body)

	if err != nil {
		return err
	}

	return json.Unmarshal(rawBody, &response)
}
