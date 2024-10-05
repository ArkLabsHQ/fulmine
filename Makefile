.PHONY: build build-templates clean cov help intergrationtest lint run test vet proto proto-lint all build clean test deps icon bundle run

## build: build for all platforms
build:
	@echo "Building ark-node binary..."
	@bash ./scripts/build

## build-desktop: build for desktop with system tray
build-desktop:
	@echo "Building ark-node binary for desktop..."
	@bash ./scripts/build-desktop

## build-mac-arm64: build for Mac ARM64 (Apple Silicon)
build-mac-arm64:
	@echo "Building ark-node binary for Mac ARM64..."
	@$(SCRIPTS_DIR)/build-desktop darwin arm64

## build-mac-amd64: build for Mac AMD64 (Intel)
build-mac-amd64:
	@echo "Building ark-node binary for Mac AMD64..."
	@$(SCRIPTS_DIR)/build-desktop darwin amd64

## build-windows-amd64: build for Windows AMD64
build-windows-amd64:
	@echo "Building ark-node binary for Windows AMD64..."
	@$(SCRIPTS_DIR)/build-desktop windows amd64

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

## bundle-mac: build and bundle for both Mac architectures
bundle-mac: build-mac-arm64 build-mac-amd64 icon
	@echo "Bundling the application for Mac..."
	@chmod +x $(SCRIPTS_DIR)/bundle-mac
	@rm -rf "$(BUILD_DIR)/$(APP_NAME)-arm64.app"
	@$(SCRIPTS_DIR)/bundle-mac "$(APP_NAME)" "$(BINARY_NAME)" "$(ICON_OUTPUT)" "$(VERSION)" "$(BUILD_DIR)" darwin arm64
	@mv "$(BUILD_DIR)/$(APP_NAME).app" "$(BUILD_DIR)/$(APP_NAME)-arm64.app"
	@rm -rf "$(BUILD_DIR)/$(APP_NAME)-amd64.app"
	@$(SCRIPTS_DIR)/bundle-mac "$(APP_NAME)" "$(BINARY_NAME)" "$(ICON_OUTPUT)" "$(VERSION)" "$(BUILD_DIR)" darwin amd64
	@mv "$(BUILD_DIR)/$(APP_NAME).app" "$(BUILD_DIR)/$(APP_NAME)-amd64.app"
	@echo "Application bundled for both architectures: $(BUILD_DIR)/$(APP_NAME)-arm64.app and $(BUILD_DIR)/$(APP_NAME)-amd64.app"

## bundle-windows: build and bundle for Windows
bundle-windows: build-windows-amd64
	@echo "Bundling the application for Windows..."
	@chmod +x $(SCRIPTS_DIR)/bundle-windows
	@$(SCRIPTS_DIR)/bundle-windows "$(APP_NAME)" "$(BINARY_NAME)" "$(ICON_SOURCE)" "$(VERSION)" "$(BUILD_DIR)"
	@echo "Windows package created: $(BUILD_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip"

## bundle-debian: build, bundle, and create Debian package
bundle-debian: build-desktop icon
	@echo "Bundling the application for Debian..."
	@chmod +x $(SCRIPTS_DIR)/bundle-debian
	@$(SCRIPTS_DIR)/bundle-debian "$(APP_NAME)" "$(BINARY_NAME)" "$(ICON_OUTPUT)" "$(VERSION)" "$(BUILD_DIR)"
	@echo "Debian package created: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_$(ARCH).deb"

bundle-mac-app:
	@echo "Bundling the application for Mac..."
	@./scripts/bundle-mac-app

sign-mac:
	@if [ "$(GOOS)" = "darwin" ]; then \
		echo "Signing the application for Mac..."; \
		quill sign-and-notarize ./dist/ark-node-desktop_darwin_arm64/ark-node-desktop; \
	else \
		echo "Skipping Mac signing for non-Darwin OS"; \
	fi