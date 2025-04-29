# Build the web interface
FROM node:22 AS web-builder

WORKDIR /app/internal/interface/web
COPY internal/interface/web .
RUN rm -rf .parcel-cache && yarn && yarn build

# Build the Go application
FROM golang:1.24.2 AS go-builder

ARG VERSION
ARG COMMIT
ARG DATE
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY . .
# Copy the built web assets from web-builder
COPY --from=web-builder /app/internal/interface/web/static ./internal/interface/web/static
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'" -o bin/fulmine cmd/fulmine/main.go

# Final image
FROM alpine:3.20

WORKDIR /app

COPY --from=go-builder /app/bin/* /app

ENV PATH="/app:${PATH}"
ENV FULMINE_DATADIR=/app/data

# Expose volume containing all 'arkd' data
VOLUME /app/data

ENTRYPOINT [ "fulmine" ]

