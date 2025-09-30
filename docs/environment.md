# Environment Variables

Generated from `config Structure`. **Do not edit manually.**

| Variable | Default | Type | Description |
|----------|--------|------|-------------|
| `FULMINE_DATADIR` | `fulmine` | `string` | Data directory for Fulmine state |
| `FULMINE_DB_TYPE` | `sqlite` | `string` | Database backend: sqlite | badger |
| `FULMINE_GRPC_PORT` | `7000` | `uint32` | gRPC server port |
| `FULMINE_HTTP_PORT` | `7001` | `uint32` | HTTP server port |
| `FULMINE_WITH_TLS` | `false` | `bool` | Enable TLS on server |
| `FULMINE_LOG_LEVEL` | `4` | `uint32` | Log verbosity (higher = more verbose) |
| `FULMINE_ARK_SERVER` | `` | `string` | Ark server address (e.g., arkd:7070) |
| `FULMINE_ESPLORA_URL` | `` | `string` | Esplora base URL (e.g., http://chopsticks:3000) |
| `FULMINE_BOLTZ_URL` | `` | `string` | Boltz HTTP endpoint (e.g., http://boltz:9001) |
| `FULMINE_BOLTZ_WS_URL` | `` | `string` | Boltz WebSocket endpoint (e.g., ws://boltz:9002) |
| `FULMINE_UNLOCKER_TYPE` | `` | `string` | Unlocker type: file | env |
| `FULMINE_UNLOCKER_FILE_PATH` | `` | `string` | Path to unlocker file |
| `FULMINE_UNLOCKER_PASSWORD` | `` | `string` | Unlocker password (if using env unlocker) |
| `FULMINE_DISABLE_TELEMETRY` | `false` | `bool` | Disable telemetry |
| `FULMINE_NO_MACAROONS` | `false` | `bool` | Disable macaroons |
| `FULMINE_SWAP_TIMEOUT` | `120` | `uint32` | Swap timeout in seconds |
| `FULMINE_LND_URL` | `` | `string` | LND connection URL (lndconnect:// or http://host:port) |
| `FULMINE_CLN_URL` | `` | `string` | CLN connection URL (clnconnect:// or http://host:port) |
| `FULMINE_CLN_DATADIR` | `` | `string` | CLN data directory (required if not using clnconnect://) |
| `FULMINE_LND_DATADIR` | `` | `string` | LND data directory (required if not using lndconnect://) |
