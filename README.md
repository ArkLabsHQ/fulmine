# ⚡️fulmine

[![Go Version](https://img.shields.io/badge/Go-1.26.1-blue.svg)](https://golang.org/doc/go1.25)
[![GitHub Release](https://img.shields.io/github/v/release/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/releases/latest)
[![License](https://img.shields.io/github/license/ArkLabsHQ/fulmine)](https://github.com/ArkLabsHQ/fulmine/blob/main/LICENSE)
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

The following environment variables can be configured:

| Variable | Description | Default |
|----------|-------------|---------|
| `FULMINE_DATADIR` | Directory to store wallet data | `/app/data` in Docker, `~/.fulmine` otherwise |
| `FULMINE_HTTP_PORT` | HTTP port for the web UI and REST API | `7001` |
| `FULMINE_GRPC_PORT` | gRPC port for service communication | `7000` |
| `FULMINE_ARK_SERVER` | URL of the Ark server to connect to | It pre-fills with the default Ark server |
| `FULMINE_ESPLORA_URL` | URL of the Esplora server to connect to | It pre-fills with the default Esplora server |
| `FULMINE_UNLOCKER_TYPE` | Type of unlocker to use for auto-unlock (`file` or `env`) | Not set by default (no auto-unlock) |
| `FULMINE_UNLOCKER_FILE_PATH` | Path to the file containing the wallet password (when using `file` unlocker) | Not set by default |
| `FULMINE_UNLOCKER_PASSWORD` | Password string to use for unlocking (when using `env` unlocker) | Not set by default |
| `FULMINE_BOLTZ_URL` | URL of the custom Boltz backend to connect to for swaps | Not set by default |
| `FULMINE_BOLTZ_WS_URL` | URL of the custom Boltz WebSocket backend to connect to for swap events | Not set by default |
| `FULMINE_DISABLE_TELEMETRY` | Opt out of telemetry logs | False by default | 

When using Docker, you can set these variables using the `-e` flag:

```bash
docker run -d \
  --name fulmine \
  -p 7001:7001 \
  -e FULMINE_HTTP_PORT=7001 \
  -e FULMINE_ARK_SERVER="https://server.example.com" \
  -e FULMINE_ESPLORA_URL="https://mempool.space/api" \
  -e FULMINE_UNLOCKER_TYPE="file" \
  -e FULMINE_UNLOCKER_FILE_PATH="/app/password.txt" \
  -v fulmine-data:/app/data \
  -v /path/to/password.txt:/app/password.txt \
  ghcr.io/arklabshq/fulmine:latest
```

### 🔑 Auto-Unlock Feature

Fulmine supports automatic wallet unlocking on startup, which is useful for unattended operation or when running as a service. Two methods are available:

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

⚠️ **Security Warning**: When using the auto-unlock feature, ensure your password is stored securely:
- For file-based unlocking, use appropriate file permissions (chmod 600)
- For environment-based unlocking, be cautious about environment variable visibility
- Consider using Docker secrets or similar tools in production environments

## 📚 API Documentation

### ⚠️ Security Notice

**IMPORTANT**: The REST API and gRPC interfaces are currently **not protected** by authentication. This is a known limitation and is being tracked in [issue #98](https://github.com/ArkLabsHQ/fulmine/issues/98). 

**DO NOT** expose these interfaces over the public internet until authentication is implemented. The interfaces should only be accessed from trusted networks or localhost.

While the wallet seed is encrypted using AES-256 with a password that the user set, the API endpoints themselves are not protected.

### 🔌 API Interfaces

Fulmine provides three main interfaces:

1. **Web UI** - Available at [http://localhost:7001](http://localhost:7001) by default
2. **REST API** - Available at [http://localhost:7001/api](http://localhost:7001/api)
3. **gRPC Service** - Available at `localhost:7000`

### 🔑 Wallet Setup & Basic Usage

Before using any wallet-dependent features, you need to set up and unlock your wallet.

1. Generate Seed

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
        -d '{"private_key": "<hex or nsec>", "password": "<strong password>", "server_url": "https://server.example.com"}'
   ```

3. Unlock Wallet

   ```sh
   curl -X POST http://localhost:7001/api/v1/wallet/unlock \
        -H "Content-Type: application/json" \
        -d '{"password": "<strong password>"}'
   ```

4. Lock Wallet

   ```sh
   curl -X POST http://localhost:7001/api/v1/wallet/lock \
        -H "Content-Type: application/json"
   ```

5. Get Wallet Status

   ```sh
   curl -X GET http://localhost:7001/api/v1/wallet/status
   ```

6. Get Arkade Address

   ```sh
   curl -X GET http://localhost:7001/api/v1/address
   ```

7. Get Onboard Address

   ```sh
   curl -X GET http://localhost:7001/api/v1/onboard
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

### 🔔 Notification API

> **Note:** Wallet setup is not required to use the Notification API.

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

   Open a server-sent event stream to receive real-time notifications for all subscribed addresses. Each event contains the affected addresses, newly received VTXOs (`new_vtxos`), and spent VTXOs (`spent_vtxos`).

   ```sh
   curl -X GET http://localhost:7001/api/v1/notifications
   ```

### ⚡ VHTLC API

> **Note:** Wallet setup is required before using the VHTLC APIs.

Virtual Hash Time-Locked Contracts (VHTLCs) are Arkade-native HTLCs that live off-chain. They enable atomic swaps and conditional payments without touching the base layer.

1. Create VHTLC

   Computes a VHTLC address from:
   * a preimage hash
   * sender or receiver pubkeys. The missing key (depending on wether fulmine has to fund or claim the VHTLC) is added by fulmine using one of its internal wallet.
   * optional locktimes, if not provided fulmine uses the following default values:
      - `refund_locktime`: 24hrs. This is the offchain (absolute) locktime the sender must wait to refund the VHTLC offchain (can be expressed in blocks only on regtest)
      - `unilateral_claim_delay`: 8 mins. This is the locktime the receiver has to wait to claim the VHTLC after it's been unrolled onchain
      - `unilateral_refund_delay`: 16 mins. This is the locktime sender and Boltz have to wait to refund the VHTLC collaboratively after it's been unrolled onchain
      - `unilateral_refund_without_receiver_delay`: 32 mins. This is the locktime the sender has to wait to refund alone the VHTLC after it's been unrolled onchain

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc \
        -H "Content-Type: application/json" \
        -d '{
          "preimage_hash": "<hex preimage hash>",
          "sender_pubkey": "<hex sender pubkey>",
          "receiver_pubkey": "<hex receiver pubkey>",
          "refund_locktime": 1024,
          "unilateral_claim_delay": {"type": "LOCKTIME_TYPE_SECONDS", "value": 2048},
          "unilateral_refund_delay": {"type": "LOCKTIME_TYPE_SECONDS", "value": 4096},
          "unilateral_refund_without_receiver_delay": {"type": "LOCKTIME_TYPE_SECONDS", "value": 8192}
        }'
   ```

   Returns: VHTLC `id`, `address`, `claim_pubkey`, `refund_pubkey`, `server_pubkey`, `swap_tree`, and the resolved locktime values.  
   The `vhtlc_id` is the sha256 hash of preimage hash + sender ec pubkey + receiver ec pubkey.

2. List VHTLCs

   Returns VHTLCs by their  `vhtlc_id`s.

   ```sh
   curl -X GET "http://localhost:7001/api/v1/vhtlcs?vhtlc_ids=id1,id2"
   ```

3. ListVHTLC

   Returns a VHTLC by its `vhtlc_id`.

   ```sh
   curl -X GET "http://localhost:7001/api/v1/vhtlcs?vhtlc_ids=id1,id2"
   ```
   
4. Claim VHTLC

   Claims a VHTLC by revealing the preimage. Moves the funds into a regular VTXO.

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/claim \
        -H "Content-Type: application/json" \
        -d '{"vhtlc_id": "<vhtlc id>", "preimage": "<hex preimage>"}'
   ```

   Returns: `{ "claim_txid": "<txid>" }`

5. Settle VHTLC

   Settles a VHTLC via either the claim path (reveal preimage) or the collaborative refund path (delegate params).

   Claim path:
   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/settle \
        -H "Content-Type: application/json" \
        -d '{"vhtlc_id": "<vhtlc id>", "claim": {"preimage": "<hex preimage>"}}'
   ```

   Refund path:
   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/settle \
        -H "Content-Type: application/json" \
        -d '{
          "vhtlc_id": "<vhtlc id>",
          "refund": {
            "delegate_params": {
              "signed_intent_proof": "<base64>",
              "intent_message": "<json string>",
              "partial_forfeit_tx": "<base64 psbt>"
            }
          }
        }'
   ```

   Returns: `{ "commitment_txid": "<txid>" }`

6. Refund VHTLC Without Receiver

   Unilaterally refunds a VHTLC after the timeout has expired, without requiring the receiver's cooperation.

   ```sh
   curl -X POST http://localhost:7001/api/v1/vhtlc/refundWithoutReceiver \
        -H "Content-Type: application/json" \
        -d '{"vhtlc_id": "<vhtlc id>"}'
   ```

   Returns: `{ "refund_txid": "<txid>" }`

### 🤝 Delegate API

> **Note:** Wallet setup is not required to use the Delegator API.

Fulmine can act as a delegate: it monitors VTXO expiry on behalf of clients and automatically refreshes them before they expire. Clients submit a signed intent and pre-signed forfeit transactions; Fulmine handles the rest.

1. Get Delegate Info

   Returns the delegate's pubkey (to include in VTXO scripts), service fee, and fee address.

   ```sh
   curl -X GET http://localhost:7001/api/v1/delegator/info
   ```

   Returns: `{ "pubkey": "<hex>", "fee": "<amount>", "delegator_address": "<ark address>" }`

2. Delegate

   Submit a delegation request. The `intent.message` is a stringified JSON describing the VTXOs to refresh. The `intent.proof` is a partially signed PSBT (base64). `forfeit_txs` are partially signed forfeit transactions (base64), one per VTXO input.

   ```sh
   curl -X POST http://localhost:7001/api/v1/delegate \
        -H "Content-Type: application/json" \
        -d '{
          "intent": {
            "message": "{\"vtxos\": [...]}",
            "proof": "<base64 psbt>"
          },
          "forfeit_txs": ["<base64 psbt>"],
          "reject_replace": false
        }'
   ```

3. List Delegates

   Returns delegate tasks filtered by status, paginated.

   ```sh
   curl -X GET "http://localhost:7001/api/v1/delegates?status=<pending|completed|failed>&limit=10&offset=0"
   ```

Note: Replace `http://localhost:7001` with the appropriate host and port where your Fulmine is running.

For more detailed information about request and response structures, please refer to the proto files in the `api-spec/protobuf/fulmine/v1/` directory.

## 👨‍💻 Development

To get started with fulmine development you need Go `1.26.1` or higher and Node.js `18.17.1` or higher.

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

1. **Fork the repository** and create your branch from `main`
2. **Install dependencies**: `go mod download`
3. **Make your changes** and ensure tests pass: `make test`4. **Run the linter** to ensure code quality: `make lint`
4. **Submit a pull request**

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
