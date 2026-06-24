# Regtest environment

Fulmine's integration tests and local end-to-end runs use the shared
[arkade-regtest](https://github.com/ArkLabsHQ/arkade-regtest) stack, vendored as a git submodule at
`regtest/` and driven by its zero-dependency Node CLI `regtest.mjs`. The same stack is used by the
Arkade SDKs and wallet.

## Prerequisites

- **Docker** + the **`docker compose`** plugin
- **Node.js ≥ 18** (the CLI uses only the standard library — no `npm install`)
- The submodule checked out:

```bash
git submodule update --init regtest
```

## Quick start

```bash
make regtest-up      # build the Fulmine-under-test image (fulmine:e2e) and start the stack
make integrationtest # run the e2e suite against the running stack
make regtest-down    # stop and remove the stack + volumes
make regtest-logs    # tail stack logs
```

`make regtest-up` builds this repo's `Dockerfile` as `fulmine:e2e` and sets `FULMINE_IMAGE=fulmine:e2e`
(via `.env.regtest`), so the stack runs **your local build** of Fulmine rather than a published image.

## What comes up

`regtest.mjs start --profile boltz,delegate` brings up Bitcoin Core, Fulcrum, mempool (Esplora REST
API under `/api`), NBXplorer, `arkd` + `arkd-wallet`, `boltz` + `boltz-lnd`, the counterparty `lnd`,
and two Fulmine instances built from this repo: `boltz-fulmine` and `fulmine-delegator`.

## Host endpoints

| Service                 | Host endpoint                  |
| ----------------------- | ------------------------------ |
| Fulmine gRPC            | `localhost:7004`               |
| Fulmine REST            | `localhost:7003`               |
| Fulmine web             | `localhost:7002`               |
| Fulmine delegator gRPC  | `localhost:7010`               |
| Esplora REST API        | `http://localhost:3000/api`    |

Override any default (image tags, ports, arkd tuning) in `.env.regtest` at the repo root — see
arkade-regtest's `.env.defaults` for the full list. `regtest.mjs` reads `.env.defaults`, then this
repo's `.env.regtest`, then process env (later wins).

## Useful CLI commands

```bash
node regtest/regtest.mjs faucet <address> <amountBtc> --confirm
node regtest/regtest.mjs mine [n]
node regtest/regtest.mjs arkd <args...>   # arkd server CLI inside the container
node regtest/regtest.mjs ark <args...>    # ark client CLI
```
