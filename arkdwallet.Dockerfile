# First stage: build the ark-wallet-daemon binary
FROM golang:1.24.6 AS builder

ARG VERSION
ARG TARGETOS
ARG TARGETARCH
ARG ARKD_VERSION=v0.7.0


WORKDIR /app

RUN git clone --branch ${ARKD_VERSION} --single-branch https://github.com/arkade-os/arkd.git

RUN cd arkd && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /app/bin/arkd-wallet ./cmd/arkd-wallet

# Second stage: minimal runtime image
FROM alpine:3.20

RUN apk update && apk upgrade

WORKDIR /app

COPY --from=builder /app/bin/arkd-wallet /app/

ENV PATH="/app:${PATH}"
ENV ARKD_WALLET_DATADIR=/app/data

VOLUME /app/data

ENTRYPOINT [ "arkd-wallet" ]
