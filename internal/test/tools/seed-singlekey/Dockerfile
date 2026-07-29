# Test-only image: builds the single-key datadir seeder used by the e2e harness.
#
# It must run INSIDE the arkade-regtest network, because the URLs it writes into
# the config store (arkd:7070, mempool_web) are container-internal names that the
# host cannot resolve.
#
# Build from the repo root:
#   docker build -t fulmine-seeder:e2e -f internal/test/tools/seed-singlekey/Dockerfile .
FROM golang:1.26.5 AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/seed-singlekey ./internal/test/tools/seed-singlekey

FROM alpine:3.20

COPY --from=builder /bin/seed-singlekey /usr/local/bin/seed-singlekey

ENTRYPOINT [ "seed-singlekey" ]
