# Environment Variables

Generated from `config Structure`. **Do not edit manually.**

| Variable | Default | Type | Description |
|----------|--------|------|-------------|
| `FULMINE_DATADIR` | `` | `string` | Data directory for Fulmine state (defaults to an OS-specific app data dir) |
| `FULMINE_DB_TYPE` | `sqlite` | `string` | Database backend: sqlite or badger |
| `FULMINE_GRPC_PORT` | `7000` | `uint32` | gRPC server port |
| `FULMINE_HTTP_PORT` | `7001` | `uint32` | HTTP server port |
| `FULMINE_WITH_TLS` | `false` | `bool` | Enable TLS on the server |
| `FULMINE_LOG_LEVEL` | `4` | `uint32` | Log verbosity (higher = more verbose) |
| `FULMINE_ARK_SERVER` | `` | `string` | Ark server address (e.g., arkd:7070) |
| `FULMINE_ESPLORA_URL` | `` | `string` | Esplora base URL (e.g., http://chopsticks:3000) |
| `FULMINE_BOLTZ_URL` | `` | `string` | Boltz HTTP endpoint (e.g., http://boltz:9001) |
| `FULMINE_BOLTZ_WS_URL` | `` | `string` | Boltz WebSocket endpoint (e.g., ws://boltz:9002) |
| `FULMINE_SCHEDULER_POLL_INTERVAL` | `600` | `int64` | Scheduler polling interval in seconds |
| `FULMINE_PROFILING_ENABLED` | `false` | `bool` | Enable profiling endpoints |
| `FULMINE_REFRESH_DB_INTERVAL` | `60` | `int64` | Interval in seconds to refresh the database with latest blockchain data |
| `FULMINE_DELEGATE_PORT` | `7002` | `uint32` | Delegate server port |
| `FULMINE_DELEGATE_FEE` | `0` | `uint64` | Fee the delegate charges, in satoshis |
| `FULMINE_DELEGATE_ENABLED` | `false` | `bool` | Run the delegate server |
| `FULMINE_DELEGATE_REGISTRATION_COALESCE_WINDOW` | `3600` | `int64` | Seconds to hold a ready delegate intent to coalesce it with others (0 = register immediately) |
| `FULMINE_DELEGATE_REGISTRATION_EXPIRY_MARGIN` | `1800` | `int64` | Safety margin in seconds before a VTXO's expiry by which its delegate intent must be registered |
| `FULMINE_DELEGATE_REGISTRATION_COALESCE_MAX` | `0` | `int64` | Max buffered delegate intents before an early flush (0 = unbounded) |
| `FULMINE_UNLOCKER_TYPE` | `` | `string` | Unlocker type: file or env |
| `FULMINE_UNLOCKER_FILE_PATH` | `` | `string` | Path to the unlocker password file (file unlocker) |
| `FULMINE_UNLOCKER_PASSWORD` | `` | `string` | Unlocker password (env unlocker) |
| `FULMINE_DISABLE_TELEMETRY` | `false` | `bool` | Disable telemetry |
| `FULMINE_SWAP_TIMEOUT` | `15` | `uint32` | Swap timeout in seconds |
| `FULMINE_OTEL_COLLECTOR_URL` | `` | `string` | OpenTelemetry collector URL; enables OTel export when set |
| `FULMINE_OTEL_PUSH_INTERVAL` | `10` | `int64` | OpenTelemetry metrics push interval in seconds |
| `FULMINE_PYROSCOPE_URL` | `` | `string` | Pyroscope server URL for continuous profiling when set |
| `FULMINE_NO_MACAROONS` | `false` | `bool` | Disable macaroons |
