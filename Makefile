.PHONY: build build-all build-static-assets build-templates clean cov help integrationtest lint run run-mutinynet run-2 test test-vhtlc vet proto proto-lint regtest-build regtest-up regtest-user-up regtest-down regtest-logs web-e2e

GOLANGCI_LINT ?= $(shell \
	echo "docker run --rm -v $$(pwd):/app -w /app golangci/golangci-lint:v2.9.0 golangci-lint"; \
)

define setup_env
    $(eval include $(1))
    $(eval export)
endef

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
	@go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate

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
	$(call setup_env, envs/dev.env)
	go run ./cmd/fulmine

run-2: clean build-static-assets
	$(call setup_env, envs/dev.2.env)
	go run ./cmd/fulmine

run-mutinynet: clean build-static-assets
	$(call setup_env, envs/mutinynet.env)
	go run ./cmd/fulmine

## test: runs all tests
test:
	@echo "Running all tests..."
	@go test -v -race --count=1 $(shell go list ./... | grep -v *internal/test/e2e*)
	@for gomod in $$(find ./pkg -name go.mod); do \
		moddir=$$(dirname $$gomod); \
		echo "Testing module $$moddir..."; \
		(cd $$moddir && go test -v ./...) || exit 1; \
	done

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
	@docker run --rm --volume "$(shell pwd):/workspace" --workdir /workspace bufbuild/buf lint

## regtest-build: build the Fulmine-under-test image consumed by the stack
regtest-build:
	@echo "Building Fulmine image (under test)..."
	@docker build -t fulmine:e2e .

## regtest-up: build the image and start the arkade-regtest stack + user Fulmine
regtest-up: regtest-build
	@echo "Starting arkade-regtest stack..."
	@git submodule update --init regtest
	@node regtest/regtest.mjs start --profile boltz,delegate,emulator
	@$(MAKE) regtest-user-up

## regtest-user-up: start + initialise the dedicated swap-user Fulmine
regtest-user-up:
	@echo "Starting user Fulmine (fulmine-user)..."
	@docker compose -f regtest-user.compose.yml up -d
	@node regtest-user-setup.mjs

## regtest-down: stop and remove the arkade-regtest stack + volumes + user Fulmine
regtest-down:
	@echo "Stopping arkade-regtest stack..."
	@docker rm -f fulmine-user covclaimd >/dev/null 2>&1 || true
	@node regtest/regtest.mjs clean || true

## regtest-logs: tail arkade-regtest stack logs
regtest-logs:
	@node regtest/regtest.mjs logs || docker compose -p arkade-regtest logs -f

## integrationtest: runs e2e tests (requires the arkade-regtest stack: make regtest-up)
integrationtest:
	@echo "Running e2e tests..."
	@go test -v -count=1 -timeout=20m -race -p=1 ./internal/test/e2e/...

## web-e2e: run the Playwright web-UI e2e suite (requires the stack: make regtest-up)
web-e2e:
	@echo "Running web UI e2e tests..."
	@cd web-e2e && npm install --no-audit --no-fund && npx playwright install --with-deps chromium && npx playwright test

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
