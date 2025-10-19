package esplora

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// electrumService implements the Service interface using Electrum protocol
type electrumService struct {
	client *ElectrumClient
}

// NewElectrumService creates a new Electrum-based blockchain service
func NewElectrumService(url string) Service {
	return &electrumService{
		client: NewElectrumClient(url, 10*time.Second),
	}
}

func (s *electrumService) GetBlockHeight(ctx context.Context) (int64, error) {
	height, err := s.client.GetBlockchainHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("electrum get height: %w", err)
	}
	return height, nil
}

// GetScriptHashHistory retrieves the transaction history for a script hash
func (s *electrumService) GetScriptHashHistory(ctx context.Context, scriptHash string) ([]TransactionItem, error) {
	result, err := s.client.call(ctx, "blockchain.scripthash.get_history", scriptHash)
	if err != nil {
		return nil, fmt.Errorf("get_history failed: %w", err)
	}

	var history []struct {
		Height int64  `json:"height"`
		TxHash string `json:"tx_hash"`
		Fee    int64  `json:"fee,omitempty"`
	}

	if err := json.Unmarshal(result, &history); err != nil {
		return nil, fmt.Errorf("failed to parse history: %w", err)
	}

	items := make([]TransactionItem, len(history))
	for i, h := range history {
		items[i] = TransactionItem{
			Height: h.Height,
			TxHash: h.TxHash,
			Fee:    h.Fee,
		}
	}

	return items, nil
}

// SubscribeScriptHash subscribes to notifications for a script hash
func (s *electrumService) SubscribeScriptHash(ctx context.Context, scriptHash string) (string, error) {
	result, err := s.client.call(ctx, "blockchain.scripthash.subscribe", scriptHash)
	if err != nil {
		return "", fmt.Errorf("subscribe failed: %w", err)
	}

	var status string
	if err := json.Unmarshal(result, &status); err != nil {
		return "", fmt.Errorf("failed to parse subscription status: %w", err)
	}

	return status, nil
}

// AddressToScriptHash converts a Bitcoin address to Electrum script hash
func AddressToScriptHash(address string) (string, error) {
	// This is a placeholder - proper implementation would need to:
	// 1. Decode the address to get the script pubkey
	// 2. Hash it with SHA256
	// 3. Reverse the bytes
	// For now, we'll return an error indicating this needs proper implementation
	return "", fmt.Errorf("address to scripthash conversion not yet implemented")
}

// scriptPubKeyToScriptHash converts a script pubkey to Electrum script hash
func scriptPubKeyToScriptHash(scriptPubKey []byte) string {
	hash := sha256.Sum256(scriptPubKey)
	// Reverse the hash bytes for Electrum format
	reversed := make([]byte, len(hash))
	for i := range hash {
		reversed[len(hash)-1-i] = hash[i]
	}
	return hex.EncodeToString(reversed)
}

// Close closes the Electrum connection
func (s *electrumService) Close() error {
	s.client.Close()
	return nil
}
