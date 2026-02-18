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

type TransactionStatus struct {
	Confirmed   bool
	BlockHeight uint32
	BlockHash   string
	BlockTime   uint64
}

type ExplorerClient interface {
	BroadcastTransaction(tx *wire.MsgTx) (string, error)
	GetFeeRate() (float64, error)
	GetCurrentBlockHeight() (uint32, error)
	GetTransactionStatus(txid string) (*TransactionStatus, error)
	GetTransaction(txid string) (string, error)
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

func (e explorerClient) GetCurrentBlockHeight() (uint32, error) {
	endpoint, err := url.JoinPath(e.baseURL, "blocks/tip/height")
	if err != nil {
		return 0, fmt.Errorf("failed to construct endpoint: %w", err)
	}

	resp, err := http.Get(endpoint)
	if err != nil {
		return 0, fmt.Errorf("failed to get block height: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get block height: status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var height uint32
	if _, err := fmt.Sscanf(string(body), "%d", &height); err != nil {
		return 0, fmt.Errorf("failed to parse block height: %w", err)
	}

	return height, nil
}

func (e explorerClient) GetTransactionStatus(txid string) (*TransactionStatus, error) {
	// First check if transaction exists
	endpoint, err := url.JoinPath(e.baseURL, "tx", txid)
	if err != nil {
		return nil, fmt.Errorf("failed to construct endpoint: %w", err)
	}

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &TransactionStatus{Confirmed: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get transaction: status %s", resp.Status)
	}

	// Parse transaction response to get status
	var txData struct {
		Status struct {
			Confirmed   bool   `json:"confirmed"`
			BlockHeight uint32 `json:"block_height"`
			BlockHash   string `json:"block_hash"`
			BlockTime   uint64 `json:"block_time"`
		} `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&txData); err != nil {
		return nil, fmt.Errorf("failed to parse transaction data: %w", err)
	}

	return &TransactionStatus{
		Confirmed:   txData.Status.Confirmed,
		BlockHeight: txData.Status.BlockHeight,
		BlockHash:   txData.Status.BlockHash,
		BlockTime:   txData.Status.BlockTime,
	}, nil
}

func (e explorerClient) GetTransaction(txid string) (string, error) {
	endpoint, err := url.JoinPath(e.baseURL, "tx", txid, "hex")
	if err != nil {
		return "", fmt.Errorf("failed to construct endpoint: %w", err)
	}

	resp, err := http.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to get transaction %s: %w", txid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get transaction %s failed with status %d: %s", txid, resp.StatusCode, string(body))
	}

	txHex, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read transaction response: %w", err)
	}

	return string(txHex), nil
}
