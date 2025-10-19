# Blockchain Data Service - Electrum & HTTP Implementation

This package provides a unified interface for fetching blockchain data, supporting both Electrum protocol and HTTP REST API (Esplora).

## Architecture

The package uses an abstraction pattern with a common `Service` interface implemented by two concrete types:
- **`electrumService`**: Uses Electrum JSON-RPC protocol over TCP
- **`httpService`**: Uses HTTP REST API (Esplora)

## Interface

```go
type Service interface {
    // GetBlockHeight returns the current blockchain height
    GetBlockHeight(ctx context.Context) (int64, error)
    
    // GetScriptHashHistory retrieves transaction history
    // For Electrum: pass the script hash
    // For HTTP: pass the Bitcoin address
    GetScriptHashHistory(ctx context.Context, scriptHashOrAddress string) ([]TransactionItem, error)
    
    // SubscribeScriptHash subscribes to notifications (Electrum only)
    // Returns the current status hash
    SubscribeScriptHash(ctx context.Context, scriptHashOrAddress string) (string, error)
    
    // Close closes any open connections or resources
    Close() error
}
```

## Usage

### Electrum Protocol (recommended for mainnet)
```go
svc := esplora.NewService("", "blockstream.info:700")
height, err := svc.GetBlockHeight(ctx)

// Get transaction history for a script hash
history, err := svc.GetScriptHashHistory(ctx, scriptHash)

// Subscribe to script hash notifications
status, err := svc.SubscribeScriptHash(ctx, scriptHash)
```

### HTTP Esplora API (backward compatible)
```go
svc := esplora.NewService("https://mempool.space/api", "")
height, err := svc.GetBlockHeight(ctx)

// Get transaction history for an address
history, err := svc.GetScriptHashHistory(ctx, address)

// Subscriptions not supported - returns error
```

## Configuration

Set the environment variables to configure the blockchain data source:

- **For Electrum (mainnet)**: Set `FULMINE_ELECTRUM_URL=blockstream.info:700`
- **For HTTP (mempool.space)**: Set `FULMINE_ESPLORA_URL=https://mempool.space/api`
- **For HTTP (blockstream.info)**: Set `FULMINE_ESPLORA_URL=https://blockstream.info/api`

**Note**: If both `FULMINE_ELECTRUM_URL` and `FULMINE_ESPLORA_URL` are set, Electrum takes priority.

## Testing

Run unit tests:
```bash
go test ./internal/infrastructure/esplora/...
```

Network tests are skipped by default in CI. To run them locally:
```bash
go test -v ./internal/infrastructure/esplora/... -timeout 30s
```

## Implementation Details

### Electrum Service (`electrumService`)
- JSON-RPC over TCP with TLS support
- Automatic TLS detection based on port (700, 50002 use TLS; 50001 uses plain TCP)
- Supports methods:
  - `blockchain.headers.subscribe` - Block height queries
  - `blockchain.scripthash.get_history` - Transaction history
  - `blockchain.scripthash.subscribe` - Real-time notifications
- Newline-delimited JSON messages
- Connection pooling with automatic reconnection

### HTTP Service (`httpService`)
- Standard HTTP REST API (Esplora compatible)
- Supports:
  - Block height queries via `/blocks/tip/height`
  - Transaction history via `/address/{address}/txs`
- Subscriptions not supported (returns error)

## Electrum Protocol Reference

- Protocol documentation: https://electrumx.readthedocs.io/en/latest/protocol-basics.html
- Methods used:
  - `blockchain.headers.subscribe` - Returns current block header with height
  - `blockchain.scripthash.get_history` - Returns transaction history for a script hash
  - `blockchain.scripthash.subscribe` - Subscribe to status changes
- Common ports:
  - Port 700: SSL/TLS (blockstream.info mainnet)
  - Port 50002: SSL/TLS (standard Electrum)
  - Port 50001: TCP plain (not recommended)

## Script Hash Conversion

Electrum uses script hashes instead of addresses. The script hash is calculated as:
1. Take the scriptPubKey bytes
2. Calculate SHA256 hash
3. Reverse the bytes
4. Hex encode the result
