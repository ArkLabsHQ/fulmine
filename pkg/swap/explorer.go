package swap

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/btcsuite/btcd/wire"
)

type ExplorerClient interface {
	BroadcastTransaction(tx *wire.MsgTx) (string, error)
	GetFeeRate() (float64, error)
}

type explorerClient struct {
	baseURL string
	client  *http.Client
}

func NewExplorerClient(baseURL string) ExplorerClient {
	return &explorerClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (e explorerClient) BroadcastTransaction(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	txHex := hex.EncodeToString(buf.Bytes())

	url := fmt.Sprintf("%s/tx", e.baseURL)
	resp, err := e.client.Post(url, "text/plain", bytes.NewReader([]byte(txHex)))
	if err != nil {
		return "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("broadcast failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response (should be txid)
	txidBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read broadcast response: %w", err)
	}

	txid := string(txidBytes)
	if txid == "" {
		// If no txid returned, compute from transaction
		txid = tx.TxHash().String()
	}

	return txid, nil
}

func (e explorerClient) GetFeeRate() (float64, error) {
	endpoint, err := url.JoinPath(e.baseURL, "fee-estimates")
	if err != nil {
		return 0, err
	}

	resp, err := e.client.Get(endpoint)
	if err != nil {
		return 0, err
	}
	// nolint:all
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get fee rate: %s", resp.Status)
	}

	var response map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, err
	}

	if len(response) == 0 {
		return 1, nil
	}

	if rate, ok := response["1"]; ok && rate > 0 {
		return rate, nil
	}
	return 1, nil
}
