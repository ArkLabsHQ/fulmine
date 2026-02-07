FROM golang:1.25.5-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -o /out/mock-boltz ./internal/test/mockboltz

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/mock-boltz /app/mock-boltz
EXPOSE 9001
ENTRYPOINT ["/app/mock-boltz"]
