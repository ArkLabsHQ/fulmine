# Swap CLI REPL

Fulmine ships a tiny interactive client in `cmd/swap` that wires the
example `SwapExampleClient` into a REPL. It is meant for demos, manual testing,
and learning how the swap handler behaves without building a full UI.

## Prerequisites

- Go toolchain (1.25+ to match the top-level `go.mod`)
- Reachable ARK gRPC server + explorer instance
- Reachable Boltz REST API + WebSocket endpoint
- Optional: a funded wallet on the ARK node so swaps can succeed

You will need to know the following URLs ahead of time:

| Flag | Description |
| ---- | ----------- |
| `--server-url` | ARK gRPC endpoint (e.g. `localhost:7070`) |
| `--explorer-url` | Explorer base URL used for polling |
| `--boltz-url` | Boltz REST base URL (e.g. `https://boltz.exchange/api`) |
| `--boltz-ws-url` | Boltz WebSocket endpoint for status updates |

## Building / Running

From the repository root:

```bash
go run ./cmd/swap \
  --server-url=http://localhost:7070 \
  --explorer-url=http://localhost:3000 \
  --boltz-url=https://boltz.test/api \
  --boltz-ws-url=wss://boltz.test/ws
```

The flags also accept environment-specific values (TLS, ports, etc.). You can
override two additional knobs:

- `--swap-timeout` (seconds, default `3600`) – forwarded to `SwapHandler`
- `--command-timeout` (Go duration, default `2m`) – deadline per REPL command

## Using the REPL

Once the binary connects it drops you into a prompt:

```
Interactive Fulmine swap REPL.
Type 'help' to list commands or 'exit' to quit.
exampleswap>
```

Commands are whitespace-delimited; everything after the command name is parsed
as arguments. Unknown commands return a helpful error without exiting.

### Command reference

| Command | Args | Description |
| ------- | ---- | ----------- |
| `get-invoice <sats>` | Amount in satoshis | Creates a receive-side swap invoice and prints the BOLT11 string. |
| `pay-invoice <bolt11>` | Lightning invoice | Pays the invoice via Boltz, prints the swap id + status. |
| `get-address` | none | Requests fresh on-chain + off-chain receive addresses. |
| `balance` | none | Forces a quick settle and prints total off-chain balance. |
| `help` | none | Displays the command summary above. |
| `exit` / `quit` | none | Leaves the REPL. |

Each command runs inside its own context with the `--command-timeout` deadline.
If a backend hangs longer than the timeout you will see a context error; simply
retry once the dependency recovers.

### Example session

```
exampleswap> get-address
On-chain address:  bc1p...
Off-chain address: ark1...

exampleswap> pay-invoice lnbc1.....
Swap e8c1f6... successful (status: 2)

exampleswap> balance
Off-chain balance: 125000 sats
```

## Troubleshooting

- **Permission errors**: The REPL writes only to the temp dir created by the
  Ark SDK. Make sure the current user can create directories under `$TMPDIR`.
- **Timeouts**: Increase `--command-timeout` when connecting over slow links
  or when the explorer/Boltz endpoints are deliberately throttled.
- **Sandbox issues**: If you use `go run`, your Go toolchain must have access
  to the network destinations listed above. CLI networking matches whatever
  policy the host enforces.
