package esplora

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// ElectrumClient implements the Electrum protocol for blockchain queries
type ElectrumClient struct {
	address string
	conn    net.Conn
	mu      sync.Mutex
	reqID   uint64
	timeout time.Duration
}

// ElectrumRequest represents a JSON-RPC request
type ElectrumRequest struct {
	ID     uint64        `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// ElectrumResponse represents a JSON-RPC response
type ElectrumResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *ElectrumError  `json:"error,omitempty"`
}

// ElectrumError represents an error in the response
type ElectrumError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewElectrumClient creates a new Electrum client
func NewElectrumClient(address string, timeout time.Duration) *ElectrumClient {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &ElectrumClient{
		address: address,
		timeout: timeout,
	}
}

// connect establishes a connection to the Electrum server
func (c *ElectrumClient) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	dialer := &net.Dialer{
		Timeout: c.timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.address, err)
	}

	c.conn = conn
	return nil
}

// disconnect closes the connection
func (c *ElectrumClient) disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// call makes a JSON-RPC call to the Electrum server
func (c *ElectrumClient) call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	if err := c.connect(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.reqID++
	reqID := c.reqID
	c.mu.Unlock()

	request := ElectrumRequest{
		ID:     reqID,
		Method: method,
		Params: params,
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Electrum protocol expects newline-delimited JSON
	requestBytes = append(requestBytes, '\n')

	// Set write deadline
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		c.disconnect()
		return nil, fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Send request
	if _, err := c.conn.Write(requestBytes); err != nil {
		c.disconnect()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Set read deadline
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		c.disconnect()
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	// Read response
	reader := bufio.NewReader(c.conn)
	responseBytes, err := reader.ReadBytes('\n')
	if err != nil {
		c.disconnect()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response ElectrumResponse
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("electrum error %d: %s", response.Error.Code, response.Error.Message)
	}

	if response.ID != reqID {
		return nil, fmt.Errorf("response ID mismatch: expected %d, got %d", reqID, response.ID)
	}

	return response.Result, nil
}

// GetBlockchainHeight fetches the current blockchain height
func (c *ElectrumClient) GetBlockchainHeight(ctx context.Context) (int64, error) {
	result, err := c.call(ctx, "blockchain.headers.subscribe")
	if err != nil {
		return 0, fmt.Errorf("blockchain.headers.subscribe failed: %w", err)
	}

	// Parse the result which should be a header object
	var header struct {
		Height int64 `json:"height"`
	}

	if err := json.Unmarshal(result, &header); err != nil {
		return 0, fmt.Errorf("failed to parse header: %w", err)
	}

	return header.Height, nil
}

// Close closes the connection
func (c *ElectrumClient) Close() {
	c.disconnect()
}
