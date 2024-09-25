.PHONY: build build-templates clean cov help intergrationtest lint run test vet proto proto-lint all build clean test deps icon bundle run

## build: build for all platforms
build:
	@echo "Building ark-node binary..."
	@bash ./scripts/build

## build-desktop: build for desktop with system tray
build-desktop:
	@echo "Building ark-node binary for desktop..."
	@bash ./scripts/build-desktop

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
run: clean
	@echo "Running ark-node in dev mode..."
	@export ARK_NODE_PORT=7000; \
	go run ./cmd/ark-node/headless

## run: run in dev mode
run-bob: clean
	@echo "Running ark-node in dev mode..."
	@export ARK_NODE_PORT=7001; \
	export ARK_NODE_DATADIR="./tmp"; \
	go run ./cmd/ark-node/headless

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
	@buf generate

## proto-lint: lint protos
proto-lint:
	@echo "Linting protos..."
	@buf lint

# Variables
BINARY_NAME := ark-node-desktop
APP_NAME := ArkNode
VERSION := 1.0.0
BUILD_DIR := build
SCRIPTS_DIR := scripts
ICON_SOURCE := icon.png
ICON_NAME := icon
ICON_OUTPUT := $(BUILD_DIR)/$(ICON_NAME).icns

icon: $(ICON_OUTPUT)

$(ICON_OUTPUT): $(ICON_SOURCE)
	@echo "Creating macOS icon..."
	@mkdir -p $(BUILD_DIR)
	@chmod +x $(SCRIPTS_DIR)/icons
	@$(SCRIPTS_DIR)/icons $(ICON_SOURCE) $(BUILD_DIR)
	@echo "Checking for icon file: $(ICON_OUTPUT)"
	@ls -l $(BUILD_DIR)
	@if [ ! -f $(ICON_OUTPUT) ]; then \
		echo "Error: Icon file not created at $(ICON_OUTPUT)"; \
		exit 1; \
	fi

bundle: build-desktop icon
	@echo "Bundling the application..."
	@chmod +x $(SCRIPTS_DIR)/bundle
	@$(SCRIPTS_DIR)/bundle "$(APP_NAME)" "$(BINARY_NAME)" "$(ICON_OUTPUT)" "$(VERSION)" "$(BUILD_DIR)"
	@echo "Application bundled: $(BUILD_DIR)/$(APP_NAME).app"

## run-mac: build, bundle, and run the macOS application
run-mac: bundle
	@echo "Running $(APP_NAME) for macOS..."
	@open "$(BUILD_DIR)/$(APP_NAME).app"
