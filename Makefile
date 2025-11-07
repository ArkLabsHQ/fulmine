.PHONY: build build-all build-static-assets build-templates clean cov help integrationtest lint run run-cln test test-vhtlc vet proto proto-lint up-test-env setup-arkd down-test-env

GOLANGCI_LINT ?= $(shell \
	echo "docker run --rm -v $$(pwd):/app -w /app golangci/golangci-lint:v2.5.0 golangci-lint"; \
)

build-static-assets: build-templates
	@echo "Generating static assets..."
	@cd internal/interface/web && rm -rf .parcel-cache && yarn && yarn build
	@cd ../../..

## build: build for your platform
build: build-static-assets
	@echo "Building fulmine binary..."
	@bash ./scripts/build

## build-all: build for all platforms
build-all: build-static-assets
	@echo "Building fulmine binary for all archs..."
	@bash ./scripts/build-all

## build-templates: build html templates for embedded frontend
build-templates:
	@echo "Building templates..."
	@go run github.com/a-h/templ/cmd/templ@latest generate		

## clean: cleans the binary
clean:
	@echo "Cleaning..."
	@go clean

## cov: generates coverage report
cov:
	@echo "Coverage..."
	@go test -cover ./...

## help: prints this help message
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## lint: lint codebase
lint:
	@echo "Linting code..."
	@$(GOLANGCI_LINT) run --fix --tests=false

## run: run in dev mode
run: clean build-static-assets
	@echo "Running fulmine in dev mode..."
	@export FULMINE_DATADIR=./datadir; \
	export FULMINE_NO_MACAROONS=true; \
	export FULMINE_LOG_LEVEL=5; \
	export FULMINE_SCHEDULER_POLL_INTERVAL=10; \
	export FULMINE_DISABLE_TELEMETRY=true; \
	export FULMINE_SWAP_TIMEOUT=15; \
	export FULMINE_BOLTZ_URL=http://localhost:9001; \
    export FULMINE_BOLTZ_WS_URL=ws://localhost:9004; \
	go run ./cmd/fulmine

run-2: clean build-static-assets
	@echo "Running fulmine in dev mode with CLN support..."
	@export FULMINE_DATADIR=./datadir-2; \
	export FULMINE_NO_MACAROONS=true; \
	export FULMINE_LOG_LEVEL=5; \
	export FULMINE_SCHEDULER_POLL_INTERVAL=10; \
	export FULMINE_DISABLE_TELEMETRY=true; \
	export FULMINE_SWAP_TIMEOUT=15; \
	export FULMINE_BOLTZ_URL=http://localhost:9001; \
    export FULMINE_BOLTZ_WS_URL=ws://localhost:9004; \
	export FULMINE_GRPC_PORT=7008; \
	export FULMINE_HTTP_PORT=7009; \
	go run ./cmd/fulmine

## test: runs all tests
test:
	@echo "Running all tests..."
		@go test -v -race --count=1 $(shell go list ./... | grep -v *internal/test/e2e*)


## test-vhtlc: runs tests for the VHTLC package
test-vhtlc:
	@echo "Running VHTLC tests..."
	@cd pkg/vhtlc && go test -v

## vet: code analysis
vet:
	@echo "Running code analysis..."
	@go vet ./...
	
## proto: compile proto stubs
proto: proto-lint
	@echo "Compiling stubs..."
	@docker run --rm --volume "$(shell pwd):/workspace" --workdir /workspace bufbuild/buf generate

## proto-lint: lint protos
proto-lint:
	@echo "Linting protos..."
	@docker run --rm --volume "$(shell pwd):/workspace" --workdir /workspace bufbuild/buf lint --exclude-path ./api-spec/protobuf/cln

pull-test-env:
	@echo "Pulling latest base images..."
	@docker compose -f test.docker-compose.yml pull
	@docker compose -f boltz.docker-compose.yml pull

build-test-env: pull-test-env
	@echo "Building test environment..."
	@docker compose -f test.docker-compose.yml build --no-cache
	@docker compose -f boltz.docker-compose.yml build --no-cache

## up-test-env: starts test environment
up-test-env:
	@echo "Starting test environment..."
	@docker compose -f test.docker-compose.yml up -d
	@docker compose -f boltz.docker-compose.yml up -d

## setup-arkd: sets up the ARK server
setup-test-env:
	@go run ./internal/test/e2e/setup/arkd/setup_ark.go
	@go run ./internal/test/e2e/setup/fulmine/setup_fulmine.go

## down-test-env: stops test environment
down-test-env:
	@echo "Stopping test environment..."
	@docker compose -f test.docker-compose.yml down
	@docker compose -f boltz.docker-compose.yml down -v

## integrationtest: runs e2e tests
integrationtest:
	@echo "Running e2e tests..."
	@go test -v -count=1 -timeout=20m -race -p=1 ./internal/test/e2e/...

# --- SQLite and SQLC commands ---

# Path to the database directory (change as needed)
DB_PATH?=./data

## mig_file: creates SQLite migration file (eg. make FILE=init mig_file)
mig_file:
	@migrate create -ext sql -dir ./internal/infrastructure/db/sqlite/migration/ $(FILE)

## mig_up: apply up migration
mig_up:
	@echo "migration up..."
	@migrate -database "sqlite://$(DB_PATH)/sqlite.db" -path ./internal/infrastructure/db/sqlite/migration/ up

## mig_down: apply down migration
mig_down:
	@echo "migration down..."
	@migrate -database "sqlite://$(DB_PATH)/sqlite.db" -path ./internal/infrastructure/db/sqlite/migration/ down

## mig_down_yes: apply down migration without prompt
mig_down_yes:
	@echo "migration down..."
	@"yes" | migrate -database "sqlite://$(DB_PATH)/sqlite.db" -path ./internal/infrastructure/db/sqlite/migration/ down

## vet_db: check if mig_up and mig_down are ok
vet_db: mig_up mig_down_yes
	@echo "vet db migration scripts..."

## sqlc: generate Go code from SQLC
sqlc:
	@echo "gen sql..."
	cd ./internal/infrastructure/db/sqlite; sqlc generate
