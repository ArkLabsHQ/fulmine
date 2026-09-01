# ⚡️fulmine

[![Go Version](https://img.shields.io/badge/Go-1.26.2-blue.svg)](https://golang.org/doc/go1.26)
[![GitHub Release](https://img.shields.io/github/v/release/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/releases/latest)
[![License](https://img.shields.io/github/license/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/blob/master/LICENSE)
[![GitHub Stars](https://img.shields.io/github/stars/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/stargazers)
[![GitHub Issues](https://img.shields.io/github/issues/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/issues)

![fulmine-og-v2](https://github.com/user-attachments/assets/8d59879d-727b-4aa7-8a9f-4d696406c6cf)


Fulmine is a Bitcoin wallet daemon built on [Arkade](https://arkadeos.com). It can be used as a general-purpose Arkade wallet or as an infrastructure node for Arkade-native services — such as serving VHTLCs or acting as a delegate for automated VTXO refresh.

## 🚀 Usage

### 🐳 Using Docker (Recommended)

The easiest way to run fulmine is using Docker. Make sure you have [Docker](https://docs.docker.com/get-docker/) installed on your machine.

```bash
docker run -d \
  --name fulmine \
  -p 7000:7000 \
  -p 7001:7001 \
  -v fulmine-data:/app/data \
  ghcr.io/arklabshq/fulmine:latest
```

Port `7001` serves the web UI and REST API; port `7000` serves the gRPC API. If you intend to run a delegate, also publish the delegate port — see [Running as a Delegate](#-running-as-a-delegate).

Once the container is running, you can access the web UI at [http://localhost:7001](http://localhost:7001).

To view logs:

```bash
docker logs -f fulmine
```

To stop the container:

```bash
docker stop fulmine
```

To update to the latest version:

```bash
docker pull ghcr.io/arklabshq/fulmine:latest
docker stop fulmine && docker rm fulmine
docker run -d \
  --name fulmine \
  -p 7000:7000 \
  -p 7001:7001 \
  -v fulmine-data:/app/data \
  ghcr.io/arklabshq/fulmine:latest
```

### 💻 Using the Binary

Alternatively, you can download the latest release from the [releases page](https://github.com/ArkLabsHQ/fulmine/releases) for your platform. After downloading:

1. Extract the binary
2. Make it executable (on Linux/macOS): `chmod +x fulmine`
3. Run the binary: `./fulmine`

### 🔧 Environment Variables

All settings are read from environment variables prefixed with `FULMINE_`. The most common ones are listed below; for the complete auto-generated list see [`docs/environment.md`](docs/environment.md), or the source of truth, [`internal/config/config.go`](internal/config/config.go).

#### Core

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_DATADIR` | Directory to store wallet, database and macaroon data | `/app/data` in Docker; otherwise an OS-specific app dir (`~/.fulmine` on Linux, `~/Library/Application Support/Fulmine` on macOS, `%LOCALAPPDATA%\Fulmine` on Windows) |
| `FULMINE_HTTP_PORT` | HTTP port for the web UI and REST API | `7001` |
| `FULMINE_GRPC_PORT` | gRPC port for service communication | `7000` |
| `FULMINE_DB_TYPE` | Database backend: `sqlite` or `badger` | `sqlite` |
| `FULMINE_LOG_LEVEL` | Log verbosity (logrus levels: `4` = info, `5` = debug, `6` = trace) | `4` |
| `FULMINE_ARK_SERVER` | URL of the Ark server to connect to. Optional — it can also be set when creating the wallet | Not set |
| `FULMINE_ESPLORA_URL` | URL of the Esplora-compatible chain API to connect to. Optional | Not set |
| `FULMINE_NO_MACAROONS` | Disable macaroon authentication on the API (see [Authentication](#-authentication)) | `false` (auth enabled) |

#### Auto-unlock (see [Auto-Unlock Feature](#-auto-unlock-feature))

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_UNLOCKER_TYPE` | Auto-unlock method: `file` or `env` | Not set (no auto-unlock) |
| `FULMINE_UNLOCKER_FILE_PATH` | Path to a file containing the wallet password (when using the `file` unlocker) | Not set |
| `FULMINE_UNLOCKER_PASSWORD` | Wallet password (when using the `env` unlocker) | Not set |

#### Auto-init (see [Auto-Init Feature](#-auto-init-feature))

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_AUTO_INIT` | Create and unlock the wallet automatically on first boot; requires an unlocker and `FULMINE_ARK_SERVER` | `false` |
| `FULMINE_MNEMONIC` | 12-word BIP39 mnemonic to restore during auto-init; omit to generate a new one | Not set |
| `FULMINE_MNEMONIC_FILE_PATH` | Path to a file containing the mnemonic to restore during auto-init (mutually exclusive with `FULMINE_MNEMONIC`) | Not set |

#### Delegate (see [Running as a Delegate](#-running-as-a-delegate))

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_DELEGATE_ENABLED` | Run a delegate service that refreshes clients' VTXOs | `false` |
| `FULMINE_DELEGATE_PORT` | Port for the delegate API (must differ from the gRPC/HTTP ports) | `7002` |
| `FULMINE_DELEGATE_FEE` | Service fee charged per delegation, in satoshis | `0` |

#### Lightning & swaps

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_BOLTZ_URL` | URL of a custom Boltz backend for swaps | Not set |
| `FULMINE_BOLTZ_WS_URL` | URL of a custom Boltz WebSocket backend for swap events | Not set |
| `FULMINE_SWAP_TIMEOUT` | Swap timeout, in seconds | `15` |

#### Advanced

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_SCHEDULER_POLL_INTERVAL` | How often (seconds) the scheduler polls for VTXOs to refresh | `600` |
| `FULMINE_REFRESH_DB_INTERVAL` | How often (seconds) the Ark SDK refreshes its local database | `60` |
| `FULMINE_DISABLE_TELEMETRY` | Opt out of telemetry logs | `false` |
| `FULMINE_PROFILING_ENABLED` | Expose a pprof server on `:6060` | `false` |
| `FULMINE_OTEL_COLLECTOR_URL` | OpenTelemetry collector endpoint (enables OTel metrics/traces) | Not set |
| `FULMINE_OTEL_PUSH_INTERVAL` | OpenTelemetry push interval, in seconds | `10` |
| `FULMINE_PYROSCOPE_URL` | Pyroscope server URL for continuous profiling (requires OTel) | Not set |

When using Docker, you can set these variables using the `-e` flag:

```bash
docker run -d \
  --name fulmine \
  -p 7000:7000 \
  -p 7001:7001 \
  -e FULMINE_ARK_SERVER="https://server.example.com" \
  -e FULMINE_ESPLORA_URL="https://mempool.space/api" \
  -e FULMINE_UNLOCKER_TYPE="file" \
  -e FULMINE_UNLOCKER_FILE_PATH="/app/password.txt" \
  -v fulmine-data:/app/data \
  -v /path/to/password.txt:/app/password.txt \
  ghcr.io/arklabshq/fulmine:latest
```

### 🔑 Auto-Unlock Feature

Fulmine supports automatic wallet unlocking on startup, which is useful for unattended operation or when running as a service (for example, a [delegate](#-running-as-a-delegate)). Two methods are available:

1. **File-based unlocker**: Reads the wallet password from a file
   ```
   FULMINE_UNLOCKER_TYPE=file
   FULMINE_UNLOCKER_FILE_PATH=/path/to/password/file
   ```

2. **Environment-based unlocker**: Uses a password directly from an environment variable
   ```
   FULMINE_UNLOCKER_TYPE=env
   FULMINE_UNLOCKER_PASSWORD=your_wallet_password
   ```

Auto-unlock only runs if a wallet already exists; you must create the wallet once first, or let [auto-init](#-auto-init-feature) create it for you on first boot.

⚠️ **Security Warning**: When using the auto-unlock feature, ensure your password is stored securely:
- For file-based unlocking, use appropriate file permissions (chmod 600)
- For environment-based unlocking, be cautious about environment variable visibility
- Consider using Docker secrets or similar tools in production environments

### 🚀 Auto-Init Feature

With `FULMINE_AUTO_INIT=true`, Fulmine creates the wallet by itself on first boot, so a fresh instance becomes fully operational from a single command — no `genseed`/`create`/`unlock` calls and no web UI visit needed. Auto-init requires an [unlocker](#-auto-unlock-feature) (the wallet is created with the unlocker's password and unlocked right after) and `FULMINE_ARK_SERVER`.

On the very first boot, Fulmine generates a new mnemonic and **prints it to stdout exactly once**, clearly marked as a backup prompt:

```
==========================================================================

  FULMINE WALLET CREATED - BACK UP YOUR MNEMONIC NOW

      word1 word2 ... word12

  ...

==========================================================================
```

Run `docker logs <container>` right after the first start and store the mnemonic offline. It is never printed again: the data directory only holds it encrypted with your wallet password. Once it is backed up, scrub it from wherever the output was captured — container logs and any log aggregator that ingests them.

To restore an existing wallet instead of generating a new one (for example when re-provisioning a crashed machine), pass the mnemonic via `FULMINE_MNEMONIC` or, preferably, a mounted secret file via `FULMINE_MNEMONIC_FILE_PATH`. Nothing is printed in that case, and if the data directory already contains a *different* wallet, Fulmine refuses to start rather than serve the wrong identity.

⚠️ **Prefer the file path in production.** A mnemonic passed in `FULMINE_MNEMONIC` is visible to anything that can read the container's environment — `docker inspect`, `/proc/<pid>/environ`, orchestrator dashboards — for the life of the container. Fulmine drops its own copy once the wallet is up, but it cannot remove the value from the process environment. With `FULMINE_MNEMONIC_FILE_PATH` the secret stays in a file you control (`chmod 600`, a Docker/Kubernetes secret mount).

If a wallet already exists, auto-init does nothing and startup behaves exactly as before (auto-unlock only).

## 🤝 Running as a Delegate

A **delegate** is a Fulmine instance that refreshes other users' VTXOs on their behalf, so their funds don't expire while they are offline. Clients submit a signed intent plus pre-signed forfeit transactions; the delegate stores them and, when a batch starts on the Ark server close to the VTXOs' expiry, joins the batch to refresh them.

### Requirements

- **The delegate is disabled by default.** Enable it with `FULMINE_DELEGATE_ENABLED=true`.
- **A delegate needs a created _and unlocked_ wallet.** The delegate service starts when the wallet is unlocked and stops when it is locked, and it signs batch transactions with the wallet's key. For unattended operation, configure [auto-init](#-auto-init-feature) and [auto-unlock](#-auto-unlock-feature).
- The delegate listens on its own port, `FULMINE_DELEGATE_PORT` (default `7002`), which **must differ** from the gRPC and HTTP ports.
- Optionally set `FULMINE_DELEGATE_FEE` (satoshis) to require a service fee; clients must pay at least this amount to the delegate's address in their intent.

### Example

This single command brings up a fully working delegate — [auto-init](#-auto-init-feature) creates and unlocks the wallet on first boot, no follow-up API calls needed:

```bash
docker run -d \
  --name fulmine-delegate \
  --restart unless-stopped \
  -p 7000:7000 \
  -p 7001:7001 \
  -p 7002:7002 \
  -e FULMINE_AUTO_INIT=true \
  -e FULMINE_DELEGATE_ENABLED=true \
  -e FULMINE_DELEGATE_FEE=1000 \
  -e FULMINE_ARK_SERVER="https://server.example.com" \
  -e FULMINE_UNLOCKER_TYPE=file \
  -e FULMINE_UNLOCKER_FILE_PATH=/app/password.txt \
  -v fulmine-data:/app/data \
  -v /path/to/password.txt:/app/password.txt \
  ghcr.io/arklabshq/fulmine:latest
```

⚠️ **Back up before onboarding users**: the first boot prints the wallet mnemonic to stdout exactly once — run `docker logs fulmine-delegate` right away and store it offline. The mnemonic is the delegate's signing identity: clients embed the delegate's pubkey in their VTXOs, so losing it means the delegate can no longer serve any of its outstanding delegations.

To re-provision a delegate on a new machine with the same identity, add `-e FULMINE_MNEMONIC="<the 12 words>"` (or mount a secret file and set `FULMINE_MNEMONIC_FILE_PATH`) to the same command. Note that restoring the mnemonic recovers the identity and funds, but delegation tasks already accepted from clients live in the database inside the data directory — back up the `fulmine-data` volume as well if you want pending tasks to survive a machine loss.

On every restart, auto-unlock brings the delegate back up automatically.

### Endpoints

The delegate exposes two **public** endpoints (no macaroon required) on the delegate port (`7002` by default). Note that these are served at the root path, **not** under `/api`:

- `GET /v1/delegate/info` — the delegate's pubkey, fee and fee address
- `POST /v1/delegate` — submit a delegation request

See the [Delegate API](#-delegate-api) section for request/response details. To inspect the status of submitted delegation tasks, clients use the authenticated `ListDelegates` endpoint on the main API (`GET /api/v1/delegates`).

## 📚 API Documentation

### 🔐 Authentication

Fulmine protects its gRPC and REST endpoints with [macaroons](https://github.com/lightninglabs/macaroons), and **authentication is enabled by default**.

- When you create or unlock a wallet, Fulmine bakes an admin macaroon at `<datadir>/macaroons/admin.macaroon` (its root key is stored in `<datadir>/macaroons/macaroons.db`). The `admin.macaroon` grants access to every protected method.
- **REST**: send the macaroon, hex-encoded, in the `X-Macaroon` header.
- **gRPC**: send the macaroon, hex-encoded, in the `macaroon` metadata field.

Get the hex string and call a protected endpoint:

```sh
# Binary install (default datadir)
MACAROON=$(xxd -p ~/.fulmine/macaroons/admin.macaroon | tr -d '\n')

# Docker
MACAROON=$(docker exec fulmine xxd -p /app/data/macaroons/admin.macaroon | tr -d '\n')

curl -X GET http://localhost:7001/api/v1/balance -H "X-Macaroon: $MACAROON"
```

The following endpoints are **public** (no macaroon required):

- Wallet lifecycle: `genseed`, `create`, `unlock`, `lock`, `auth`, `status`, `password/change`, `wallet/restore`
- The [Delegate API](#-delegate-api) (`/v1/delegate/info` and `/v1/delegate`)

All other endpoints — balance, addresses, sending, VHTLCs, notifications, chain swaps, `ListDelegates`, etc. — require the macaroon.

To disable authentication entirely (e.g. for local development), set `FULMINE_NO_MACAROONS=true`.

> ⚠️ **Do not expose an instance with authentication disabled to the public internet.** Even with macaroons enabled, only expose the interfaces you need, and prefer a trusted network or a reverse proxy that terminates TLS (Fulmine does not terminate TLS itself yet). While the wallet seed is encrypted at rest using AES-256 with your password, the API grants full control of the wallet to any holder of the admin macaroon.

### 🔌 API Interfaces

Fulmine provides the following interfaces:

1. **Web UI** — available at [http://localhost:7001](http://localhost:7001) by default
2. **REST API** — available under [http://localhost:7001/api](http://localhost:7001/api) (e.g. `GET /api/v1/wallet/status`)
3. **gRPC Service** — available at `localhost:7000`
4. **Delegate API** — when enabled, available at `localhost:7002` (gRPC and REST on the same port). See [Running as a Delegate](#-running-as-a-delegate).

REST paths mirror the gRPC method bindings but are prefixed with `/api`. The examples below use the REST API.

### 🔑 Wallet Setup & Basic Usage

Before using any wallet-dependent feature, you need to set up and unlock your wallet. The wallet lifecycle endpoints are public, but every endpoint in this section after unlock requires the macaroon (omitted below for brevity — add `-H "X-Macaroon: $MACAROON"`).

1. Generate Seed

   Returns a new key in both hex and Nostr `nsec` form.

   ```sh
   curl -X GET http://localhost:7001/api/v1/wallet/genseed
   ```

2. Create Wallet

   Password must:
   - Be 8 chars or longer
   - Have at least one number
   - Have at least one special char

   Private key supported formats:
   - 64 chars hexadecimal
   - Nostr nsec (NIP-19)

   ```sh
   curl -X POST http://localhost:7001/api/v1/wallet/create \
        -H "Content-Type: application/json" \
        -d '{"privateKey": "<hex or nsec>", "password": "<strong password>", "serverUrl": "https://server.example.com"}'
   ```

3. Unlock Wallet

   ```sh
   curl -X POST http://localhost:7001/api/v1/wallet/unlock \
        -H "Content-Type: application/json" \
        -d '{"password": "<strong password>"}'
   ```

4. Lock Wallet

   Locks the wallet. Takes no parameters.

   ```sh
   curl -X POST http://localhost:7001/api/v1/wallet/lock \
        -H "Content-Type: application/json"
   ```

5. Get Wallet Status

   ```sh
   curl -X GET http://localhost:7001/api/v1/wallet/status
   ```

   Returns: `{ "initialized": <bool>, "synced": <bool>, "unlocked": <bool> }`

6. Get Arkade Address

   ```sh
   curl -X GET http://localhost:7001/api/v1/address
   ```

   Returns: `{ "address": "<ark address>", "pubkey": "<hex>" }`

7. Get Onboard Address

   Returns an onchain address to board the requested amount into Ark.

   ```sh
   curl -X POST http://localhost:7001/api/v1/onboard \
        -H "Content-Type: application/json" \
        -d '{"amount": <amount in sats>}'
   ```

8. Send funds offchain

   ```sh
   curl -X POST http://localhost:7001/api/v1/send/offchain \
        -H "Content-Type: application/json" \
        -d '{"address": "<ark address>", "amount": <amount in sats>}'
   ```

9. Send funds onchain

   ```sh
   curl -X POST http://localhost:7001/api/v1/send/onchain \
        -H "Content-Type: application/json" \
        -d '{"address": "<bitcoin address>", "amount": <amount in sats>}'
   ```

This is only a subset of the wallet/service API. Other endpoints include balance, transaction history, settlement, invoices (Lightning), chain swaps and VTXO queries — see the [proto and OpenAPI specs](#-full-api-reference).

### 🔔 Notification API

> **Note:** The Notification API does not need wallet keys to function, but — like all `/api` endpoints — it is macaroon-protected by default. With authentication enabled you must have created/unlocked a wallet to obtain `admin.macaroon` (or set `FULMINE_NO_MACAROONS=true`).

Fulmine can track off-chain addresses on behalf of external services and deliver notifications whenever funds are received or spent.

1. Subscribe to Addresses

   Ask Fulmine to watch one or more off-chain addresses.

   ```sh
   curl -X POST http://localhost:7001/api/v1/subscribe \
        -H "Content-Type: application/json" \
        -d '{"addresses": ["<ark address>", "<ark address>"]}'
   ```

2. Unsubscribe from Addresses

   Stop watching one or more addresses.

   ```sh
   curl -X POST http://localhost:7001/api/v1/unsubscribe \
        -H "Content-Type: application/json" \
        -d '{"addresses": ["<ark address>"]}'
   ```

3. Stream Notifications

   Open a server-sent event stream to receive real-time notifications for all subscribed addresses. Each event contains the affected addresses, newly received VTXOs (`newVtxos`), and spent VTXOs (`spentVtxos`).

   ```sh
   curl -X GET http://localhost:7001/api/v1/notifications
   ```

### ⚡ VHTLC API

> **Note:** Wallet setup is required before using the VHTLC APIs, and these endpoints are macaroon-protected.

Virtual Hash Time-Locked Contracts (VHTLCs) are Arkade-native HTLCs that live off-chain. They enable atomic swaps and conditional payments without touching the base layer.

1. Create VHTLC

   Computes a VHTLC address from:
   * a preimage hash
   * **exactly one** of `senderPubkey` or `receiverPubkey` — fulmine supplies the missing key from one of its internal wallets (depending on whether it funds or claims the VHTLC). Setting both, or neither, is rejected.
   * optional locktimes. If not provided, fulmine uses the following defaults:
      - `refundLocktime`: an absolute locktime after which the sender can refund the VHTLC off-chain. Defaults to **24 hours** from creation.
      - `unilateralClaimDelay`: how long the receiver must wait to claim on-chain after the VHTLC is unrolled. Default **512 seconds**.
      - `unilateralRefundDelay`: how long the sender and the counterparty must wait to refund collaboratively on-chain after unroll. Default **1024 seconds**.
      - `unilateralRefundWithoutReceiverDelay`: how long the sender must wait to refund alone on-chain after unroll. Default **2048 blocks** (note: this default is block-denominated, not time).

   Relative locktimes are objects of the form `{"type": "LOCKTIME_TYPE_SECOND" | "LOCKTIME_TYPE_BLOCK", "value": <number>}`.

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc \
        -H "Content-Type: application/json" \
        -d '{
          "preimageHash": "<hex preimage hash>",
          "senderPubkey": "<hex sender pubkey>",
          "refundLocktime": 1750000000,
          "unilateralClaimDelay": {"type": "LOCKTIME_TYPE_SECOND", "value": 512},
          "unilateralRefundDelay": {"type": "LOCKTIME_TYPE_SECOND", "value": 1024},
          "unilateralRefundWithoutReceiverDelay": {"type": "LOCKTIME_TYPE_BLOCK", "value": 2048}
        }'
   ```

   Returns: VHTLC `id`, `address`, `claimPubkey`, `refundPubkey`, `serverPubkey`, `swapTree`, and the resolved locktime values.
   The `id` is the sha256 hash of `preimageHash` + sender EC pubkey + receiver EC pubkey.

2. List VHTLCs

   Returns VTXOs at the VHTLC addresses identified by their `vhtlcId`s.

   ```sh
   curl -X GET "http://localhost:7001/api/v1/vhtlcs?vhtlcIds=id1&vhtlcIds=id2"
   ```

3. List a single VHTLC

   Returns the VTXOs at the VHTLC address identified by its `vhtlcId`.

   ```sh
   curl -X GET "http://localhost:7001/api/v1/vhtlc?vhtlcId=id1"
   ```

4. Claim VHTLC

   Claims a VHTLC by revealing the preimage. Moves the funds into a regular VTXO.

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/claim \
        -H "Content-Type: application/json" \
        -d '{"vhtlcId": "<vhtlc id>", "preimage": "<hex preimage>"}'
   ```

   Returns: `{ "redeemTxid": "<txid>" }`

5. Settle VHTLC

   Settles a VHTLC via either the claim path (reveal preimage) or the collaborative refund path (delegate params).

   Claim path:
   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/settle \
        -H "Content-Type: application/json" \
        -d '{"vhtlcId": "<vhtlc id>", "claim": {"preimage": "<hex preimage>"}}'
   ```

   Refund path:
   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/settle \
        -H "Content-Type: application/json" \
        -d '{
          "vhtlcId": "<vhtlc id>",
          "refund": {
            "delegateParams": {
              "signedIntentProof": "<base64>",
              "intentMessage": "<json string>",
              "partialForfeitTx": "<base64 psbt>"
            }
          }
        }'
   ```

   Returns: `{ "txid": "<txid>" }`

6. Refund VHTLC Without Receiver

   Unilaterally refunds a VHTLC after the timeout has expired, without requiring the receiver's cooperation.

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/refundWithoutReceiver \
        -H "Content-Type: application/json" \
        -d '{"vhtlcId": "<vhtlc id>"}'
   ```

   Returns: `{ "redeemTxid": "<txid>" }`

### 🤝 Delegate API

The first two endpoints are served on the **delegate port** (`7002` by default), at the root path, and are **public** (no macaroon). They are only available when the delegate is [enabled](#-running-as-a-delegate). `ListDelegates` is part of the main, authenticated API.

1. Get Delegate Info

   Returns the delegate's pubkey (to include in VTXO scripts), service fee, and fee address.

   ```sh
   curl -X GET http://localhost:7002/v1/delegate/info
   ```

   Returns: `{ "pubkey": "<hex>", "fee": "<sats>", "delegateAddress": "<ark address>", "delegatorAddress": "<ark address>" }`

   > `delegatorAddress` is a legacy alias of `delegateAddress` (same value) and will be deprecated.

2. Delegate

   Submit a delegation request. `intent.message` is a stringified Ark intent (`RegisterMessage`) describing the VTXOs to refresh, and `intent.proof` is the partially signed intent transaction (base64 PSBT). `forfeitTxs` are partially signed forfeit transactions (base64 PSBT), one per VTXO input. Set `rejectReplace` to `true` to fail rather than replace an existing pending task that shares an input.

   ```sh
   curl -X POST http://localhost:7002/v1/delegate \
        -H "Content-Type: application/json" \
        -d '{
          "intent": {
            "message": "<stringified RegisterMessage>",
            "proof": "<base64 psbt>"
          },
          "forfeitTxs": ["<base64 psbt>"],
          "rejectReplace": false
        }'
   ```

   Returns an empty object `{}` on success.

3. List Delegates

   Part of the **main** API (authenticated, on port `7001`). Returns delegate tasks filtered by status, paginated. The `status` parameter is **required** and must be one of `pending`, `completed`, `failed` or `cancelled`. `limit` defaults to `100` (max `1000`).

   ```sh
   curl -X GET "http://localhost:7001/api/v1/delegates?status=pending&limit=10&offset=0" \
        -H "X-Macaroon: $MACAROON"
   ```

Note: Replace the host and ports above with wherever your Fulmine is running.

### 📖 Full API Reference

The examples above cover the most common operations. For the complete request/response schemas of every endpoint, see:

- **Protobuf** definitions: [`api-spec/protobuf/fulmine/v1/`](api-spec/protobuf/fulmine/v1/)
- **OpenAPI / Swagger** specs: [`api-spec/openapi/swagger/fulmine/v1/`](api-spec/openapi/swagger/fulmine/v1/)

## 👨‍💻 Development

To get started with fulmine development you need Go `1.26.2` or higher and Node.js `18.17.1` or higher (the Docker image builds the web assets with Node 22).

```bash
git clone https://github.com/ArkLabsHQ/fulmine.git
cd fulmine
go mod download
make run
```

Now navigate to [http://localhost:7001/](http://localhost:7001/) to see the web UI.

### Testing

Run all unit tests:
```bash
make test
```

Run integration tests:
```bash
make build-test-env
make setup-test-env
make integrationtest
make down-test-env
```

## 🤝 Contributing

We welcome contributions to fulmine! Here's how you can help:

1. **Fork the repository** and create your branch from `master`
2. **Install dependencies**: `go mod download`
3. **Make your changes** and ensure tests pass: `make test`
4. **Run the linter** to ensure code quality: `make lint`
5. **Submit a pull request**

For major changes, please open an issue first to discuss what you would like to change.

### 🛠️ Development Commands

The Makefile contains several useful commands for development:

- `make run`: Run in development mode
- `make build`: Build the binary for your platform
- `make test`: Run unit tests
- `make lint`: Lint the codebase
- `make proto`: Generate protobuf stubs (requires Docker)

## Support

If you encounter any issues or have questions, please file an issue on our [GitHub Issues](https://github.com/ArkLabsHQ/fulmine/issues) page.

## Security

We take the security of Ark seriously. If you discover a security vulnerability, we appreciate your responsible disclosure.

Currently, we do not have an official bug bounty program. However, we value the efforts of security researchers and will consider offering appropriate compensation for significant, [responsibly disclosed vulnerabilities](./SECURITY.md).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
