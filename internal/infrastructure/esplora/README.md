# Electrum Protocol Implementation

This package now supports both HTTP Esplora API and Electrum protocol for fetching blockchain data.

## Usage

### Electrum Protocol (recommended for mainnet)
```go
svc := esplora.NewService("", "blockstream.info:700")
height, err := svc.GetBlockHeight(ctx)
```

### HTTP Esplora API (backward compatible)
```go
svc := esplora.NewService("https://mempool.space/api", "")
height, err := svc.GetBlockHeight(ctx)
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

The service prioritizes protocols in this order:
1. **Electrum** - If `electrumURL` parameter is provided, use Electrum protocol
2. **HTTP** - Otherwise, use HTTP REST API with `esploraURL` parameter

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
