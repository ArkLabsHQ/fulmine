# Environment Variables

Generated from `config.EnvSpecs()`. **Do not edit manually.**

| Variable | Default | Type | Description |
|----------|--------|------|-------------|
| `FULMINE_DATADIR` | `${HOME}/.fulmine` | `string (path)` | Data directory for Fulmine state |
| `FULMINE_DB_TYPE` | `sqlite` | `string` | Database backend: sqlite | badger |
| `FULMINE_GRPC_PORT` | `7000` | `uint32 (port)` | gRPC server port |
| `FULMINE_HTTP_PORT` | `7001` | `uint32 (port)` | HTTP server port |
| `FULMINE_WITH_TLS` | `false` | `bool` | Enable TLS on server |
| `FULMINE_LOG_LEVEL` | `4` | `uint32 (0–5)` | Log verbosity (higher = more verbose) |
| `FULMINE_ARK_SERVER` | `—` | `string (host:port)` | Ark server address (e.g., arkd:7070) |
| `FULMINE_ESPLORA_URL` | `—` | `string (URL)` | Esplora base URL (e.g., http://chopsticks:3000) |
| `FULMINE_BOLTZ_URL` | `—` | `string (URL)` | Boltz HTTP endpoint (e.g., http://boltz:9001) |
| `FULMINE_BOLTZ_WS_URL` | `—` | `string (WS URL)` | Boltz WebSocket endpoint (e.g., ws://boltz:9004) |
| `FULMINE_DISABLE_TELEMETRY` | `false` | `bool` | Disable telemetry collection |
| `FULMINE_NO_MACAROONS` | `false` | `bool` | Disable macaroon authentication (if true)<br/><em>If false, macaroons are stored under DATADIR and enforced on RPCs.</em> |
| `FULMINE_LND_URL` | `—` | `string (URL or lndconnect://)` | LND connection: lndconnect://… or https://host:port<br/><em>Cannot be set with CLN_URL. If not lndconnect://, LND_DATADIR must be set.</em> |
| `FULMINE_LND_DATADIR` | `—` | `string (path)` | Path to LND data directory (required when LND_URL is https://…) |
| `FULMINE_CLN_URL` | `—` | `string (URL or clnconnect://)` | CLN connection: clnconnect://… or https://host:port<br/><em>Cannot be set with LND_URL. If not clnconnect://, CLN_DATADIR must be set.</em> |
| `FULMINE_CLN_DATADIR` | `—` | `string (path)` | Path to CLN data directory (required when CLN_URL is https://…) |
| `FULMINE_UNLOCKER_TYPE` | `—` | `string` | Unlocker backend: file | env<br/><em>file → use UNLOCKER_FILE_PATH; env → use UNLOCKER_PASSWORD</em> |
| `FULMINE_UNLOCKER_FILE_PATH` | `—` | `string (path)` | Path to file containing password (when UNLOCKER_TYPE=file) |
| `FULMINE_UNLOCKER_PASSWORD` | `—` | `string` | Password from environment (when UNLOCKER_TYPE=env) |
