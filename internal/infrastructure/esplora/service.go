package esplora

import (
	"context"
)

// TransactionItem represents a transaction in history
type TransactionItem struct {
	Height int64  // Block height (-1 for unconfirmed)
	TxHash string // Transaction hash
	Fee    int64  // Transaction fee in satoshis
}

// Service defines the interface for blockchain data retrieval
// Both Electrum and HTTP implementations conform to this interface
type Service interface {
	// GetBlockHeight returns the current blockchain height
	GetBlockHeight(ctx context.Context) (int64, error)

	// GetScriptHashHistory retrieves transaction history for a script hash (Electrum) or address (HTTP)
	// For Electrum: pass the script hash
	// For HTTP: pass the Bitcoin address
	GetScriptHashHistory(ctx context.Context, scriptHashOrAddress string) ([]TransactionItem, error)

	// SubscribeScriptHash subscribes to notifications for a script hash (Electrum only)
	// For HTTP: returns an error as subscriptions are not supported
	// Returns the current status hash
	SubscribeScriptHash(ctx context.Context, scriptHashOrAddress string) (string, error)

	// Close closes any open connections or resources
	Close() error
}

// NewService creates a new blockchain service
// If electrumURL is provided, it uses Electrum protocol implementation
// Otherwise, it falls back to HTTP REST API (Esplora) implementation
func NewService(esploraURL, electrumURL string) Service {
	// Prioritize Electrum if provided
	if electrumURL != "" {
		return NewElectrumService(electrumURL)
	}

	// Fall back to HTTP REST API (Esplora)
	return NewHTTPService(esploraURL)
}
