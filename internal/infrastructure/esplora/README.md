# Electrum Protocol Implementation

This package now supports both HTTP Esplora API and Electrum protocol for fetching blockchain data.

## Usage

### Electrum Protocol (recommended for mainnet)
```go
svc := esplora.NewService("blockstream.info:700")
height, err := svc.GetBlockHeight(ctx)
```

### HTTP Esplora API (backward compatible)
```go
svc := esplora.NewService("https://mempool.space/api")
height, err := svc.GetBlockHeight(ctx)
```

## Configuration

Set the `FULMINE_ESPLORA_URL` environment variable:

- For Electrum (mainnet): `blockstream.info:700`
- For HTTP (mempool.space): `https://mempool.space/api`
- For HTTP (blockstream.info): `https://blockstream.info/api`

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

The service auto-detects whether to use Electrum or HTTP based on the URL format:
- URLs with `http://` or `https://` prefix use HTTP REST API
- URLs without protocol (e.g., `host:port`) use Electrum protocol

The Electrum client uses:
- JSON-RPC over TCP
- Automatic TLS detection based on port (700, 50002 use TLS; 50001 uses plain TCP)
- `blockchain.headers.subscribe` method for block height queries
- Newline-delimited JSON messages
- Connection pooling with automatic reconnection

## Electrum Protocol Reference

- Protocol documentation: https://electrumx.readthedocs.io/en/latest/protocol-basics.html
- Method used: `blockchain.headers.subscribe` - Returns the current block header with height
- Common ports:
  - Port 700: SSL/TLS (blockstream.info mainnet)
  - Port 50002: SSL/TLS (standard Electrum)
  - Port 50001: TCP plain (not recommended)
