# Web UI e2e tests (Playwright)

Browser-driven end-to-end tests for the Fulmine **web UI**, complementing the
gRPC-level Go e2e suite in `internal/test/e2e/`. The Go suite drives the daemon's
gRPC API; this one drives the parts that only run in a browser — htmx page swaps,
the client-side LN-address detection, and the chain-swap UI — against the real
arkade-regtest stack.

## Run

Requires Docker + Node >= 18 and the regtest stack running:

```bash
make regtest-up      # build fulmine:e2e + start the stack (once)
make web-e2e         # install deps + run the Playwright suite
make regtest-down    # tear the stack down when finished
```

Playwright's global setup brings up a **dedicated** `fulmine-web` container
(`regtest-web.compose.yml`) on the stack's network — separate from the Go suite's
`fulmine-user` so the two can't mutate each other's wallet — and tears it down
afterwards.

## What's covered

- **`01-init`** — wallet create/unlock through the UI, and the #438 fix: a failed
  `Setup` shows the error instead of redirecting to `/done`.
- **`02-send`** — client-side LN-address/LNURL detection, the "Send to lightning"
  label, and the Exit-on-chain vs Swap-to-BTC radios (present for BTC, absent for
  ARK).
- **`03-receive`** — Lightning-invoice generation, and "Receive via BTC" creating
  a BTC->ARK chain swap with a real boltz lockup address.

## Notes

- `FULMINE_WEB_URL` overrides the target (default `http://localhost:7019`).
- `FULMINE_WEB_KEEP=1` leaves `fulmine-web` running after the suite, for debugging.
- The specs run **serially** (one shared wallet): `01-init` creates and unlocks
  it before `02-send` / `03-receive` use it.
