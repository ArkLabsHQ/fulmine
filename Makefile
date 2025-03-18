.PHONY: build build-all build-static-assets build-templates clean cov help intergrationtest lint run run-cln test vet proto proto-lint

build-static-assets:
	@echo "Generating static assets..."
	@cd internal/interface/web && rm -rf .parcel-cache && yarn build
	@cd ../../..

## build: build for your platform
build: build-static-assets build-templates
	@echo "Building ark-node binary..."
	@bash ./scripts/build

## build-all: build for all platforms
build-all: build-templates
	@echo "Building ark-node binary for all archs..."
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

## intergrationtest: runs integration tests
integrationtest:
	@echo "Running integration tests..."
	@go test -v -count=1 -race ./... $(go list ./... | grep internal/test)

## lint: lint codebase
lint:
	@echo "Linting code..."
	@golangci-lint run --fix

## run: run in dev mode
run: clean build-static-assets build-templates
	@echo "Running ark-node in dev mode..."
	@go run ./cmd/ark-node

run-cln: clean build-templates
	@echo "Running ark-node in dev mode with CLN support..."
	@export ARK_NODE_GRPC_PORT=7008; \
	export ARK_NODE_HTTP_PORT=7009; \
	export ARK_NODE_DATADIR="./node-cln"; \
	export ARK_NODE_CLN_DATADIR="~/Library/Application Support/Nigiri/volumes/lightningd/regtest/"; \
	go run ./cmd/ark-node

## test: runs unit and component tests
test:
	@echo "Running unit tests..."
	@go test -v -count=1 -race ./... $(go list ./... | grep -v internal/test)

## vet: code analysis
vet:
	@echo "Running code analysis..."
	@go vet ./...
	
	
## proto: compile proto stubs
proto: proto-lint
	@echo "Compiling stubs..."
	@docker run --rm --volume "$(shell pwd):/workspace" --workdir /workspace buf generate

## proto-lint: lint protos
proto-lint:
	@echo "Linting protos..."
	@docker run --rm --volume "$(shell pwd):/workspace" --workdir /workspace buf lint --exclude-path ./api-spec/protobuf/cln