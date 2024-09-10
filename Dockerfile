# First image used to build the sources
FROM golang:1.23.1 AS builder

ARG VERSION
ARG COMMIT
ARG DATE
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.Date=${DATE}}'" -o bin/ark-node cmd/ark-node/main.go

# Second image, running the arkd executable
FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/bin/* /app

ENV PATH="/app:${PATH}"
ENV ARK_NODE_DATADIR=/app/data

# Expose volume containing all 'arkd' data
VOLUME /app/data

ENTRYPOINT [ "ark-node" ]
    
