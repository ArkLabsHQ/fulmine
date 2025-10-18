# Copilot Instructions for Fulmine

## Project Overview

Fulmine is a Bitcoin wallet daemon that enables swap providers and payment hubs to optimize Lightning Network channel liquidity while minimizing on-chain fees. It's built in Go and provides both a web UI and programmatic APIs (REST and gRPC) for wallet management and Lightning operations.

## Technology Stack

- **Language**: Go 1.24.6+
- **Frontend**: Node.js 18.17.1+, Templ templates, Parcel bundler
- **Protocol Buffers**: For API definitions (gRPC and REST)
- **Database**: SQLite with SQLC for type-safe queries
- **Testing**: Go standard testing library with integration tests using Docker Compose

## Architecture

The project follows a clean architecture pattern:

```
.
├── cmd/fulmine/          # Main application entry point
├── internal/             # Private application code
│   ├── config/          # Configuration management
│   ├── core/            # Core business logic
│   ├── infrastructure/  # External interfaces (DB, APIs)
│   ├── interface/       # HTTP/gRPC handlers, web UI
│   └── test/            # Integration and e2e tests
├── pkg/                 # Public libraries
│   ├── boltz/          # Boltz swap integration
│   ├── macaroon/       # Authentication (macaroon-based)
│   ├── swap/           # Swap functionality
│   └── vhtlc/          # Virtual HTLC implementation
├── api-spec/           # Protocol buffer definitions
└── docs/               # Additional documentation
```

## Development Workflow

### Building and Running

- **Development mode**: `make run` - Builds static assets and runs with debug settings
- **Build binary**: `make build` - Compiles for your platform
- **Build templates**: Templates are generated using Templ before building
- **Static assets**: Frontend assets are built with Parcel (requires yarn)

### Testing

- **Unit tests**: `make test` - Runs all unit tests excluding e2e
- **VHTLC tests**: `make test-vhtlc` - Runs VHTLC-specific tests
- **Integration tests**: Requires test environment:
  ```bash
  make build-test-env    # Build Docker images
  make up-test-env       # Start test containers
  make setup-test-env    # Initialize test environment
  make integrationtest   # Run e2e tests
  make down-test-env     # Stop test environment
  ```

### Code Quality

- **Linting**: `make lint` - Runs golangci-lint with auto-fix
- **Configuration**: `.golangci.yml` - excludes test directories and generated code
- **Code analysis**: `make vet` - Runs go vet for static analysis
- **Coverage**: `make cov` - Generates coverage report

### Protocol Buffers

- **Generate stubs**: `make proto` - Uses Docker to run buf generate
- **Lint protos**: `make proto-lint` - Validates proto files (excludes cln directory)
- **Location**: All proto files are in `api-spec/protobuf/`

## Code Conventions

### Go Code Style

1. **Follow standard Go conventions**: Use gofmt, effective Go patterns
2. **Error handling**: Always check errors, use descriptive error messages
3. **Package organization**: 
   - `internal/` for private application code
   - `pkg/` for reusable libraries
   - Keep packages focused and cohesive
4. **Testing**: Write table-driven tests when possible, use subtests for clarity
5. **Comments**: Document exported functions, types, and complex logic

### API Design

1. **Protocol Buffers**: All APIs are defined in proto files first
2. **Versioning**: APIs are versioned (e.g., `v1`)
3. **RESTful principles**: REST endpoints follow REST conventions
4. **gRPC services**: Use streaming where appropriate for real-time data

### Database

1. **Migrations**: Use golang-migrate for schema changes
   - Create: `make FILE=name mig_file`
   - Apply: `make mig_up`
   - Rollback: `make mig_down`
2. **Queries**: Use SQLC for type-safe SQL queries
   - Generate code: `make sqlc`
   - Write SQL in `.sql` files with SQLC annotations

### Frontend

1. **Templates**: Use Templ for type-safe HTML templates
2. **Assets**: Build with Parcel, stored in `internal/interface/web`
3. **API integration**: Frontend calls REST API endpoints

## Security Considerations

⚠️ **Important Security Notes**:

1. **API Authentication**: REST API and gRPC are currently NOT protected by authentication (tracked in issue #98)
2. **Wallet Security**: 
   - Wallet seeds are encrypted with AES-256
   - Passwords must be 8+ chars with at least one number and special character
3. **Auto-unlock**: When implementing features involving auto-unlock:
   - File-based: Ensure proper file permissions (chmod 600)
   - Environment-based: Be cautious about environment variable visibility
4. **Secrets Management**: Never commit secrets, use environment variables or secure vaults
5. **Disclosure**: Report security issues privately to security@arklabs.to

## Key Features and Concepts

### Wallet Management

- **Seed generation**: BIP39-compatible seed generation
- **Wallet states**: Uninitialized, locked, unlocked
- **Private key formats**: Supports hex and Nostr nsec (NIP-19)

### Payment Types

- **Offchain**: Ark protocol for instant, low-fee transfers
- **Onchain**: Standard Bitcoin transactions
- **Lightning**: Integration via VHTLCs (Virtual HTLCs)

### VHTLCs (Virtual HTLCs)

Virtual HTLCs are a key innovation in Fulmine for optimized Lightning operations:
- Located in `pkg/vhtlc/`
- Special test suite: `make test-vhtlc`
- Used for efficient channel management without constant on-chain operations

### Swap Integration

- **Boltz**: Integration for submarine swaps
- **Configuration**: Customizable backend URLs via environment variables
- **Package**: `pkg/boltz/` and `pkg/swap/`

## Environment Variables

Key configuration options (see README for complete list):

- `FULMINE_DATADIR`: Data directory location
- `FULMINE_HTTP_PORT`: Web UI/REST API port (default: 7001)
- `FULMINE_GRPC_PORT`: gRPC service port (default: 7000)
- `FULMINE_ARK_SERVER`: Ark server URL
- `FULMINE_ESPLORA_URL`: Bitcoin blockchain explorer URL
- `FULMINE_LOG_LEVEL`: Logging verbosity (5 = debug)
- `FULMINE_NO_MACAROONS`: Disable macaroon authentication (dev only)

## Common Patterns

### Making Changes

1. **Minimal changes**: Make the smallest possible modification to achieve the goal
2. **Test first**: Run existing tests before making changes
3. **Iterative development**: Build, test, lint frequently
4. **Documentation**: Update README or API docs if public interfaces change

### Adding New Features

1. Define API in protocol buffers if needed
2. Implement core logic in `internal/core/`
3. Add infrastructure layer in `internal/infrastructure/`
4. Expose via handlers in `internal/interface/`
5. Write tests covering the new functionality
6. Update documentation

### Debugging

- Use `FULMINE_LOG_LEVEL=5` for verbose logging
- Check Docker logs for test environment: `docker logs <container>`
- Frontend logs available in browser console
- Integration tests run in isolated Docker environment

## Contributing Guidelines

1. Fork and create a branch from `main`
2. Install dependencies: `go mod download`
3. Make changes and ensure tests pass: `make test`
4. Run linter: `make lint`
5. Keep commits focused and descriptive
6. Submit pull request with clear description

## Additional Resources

- **API Spec**: See `api-spec/protobuf/` for proto definitions
- **Integration Tests**: Examples in `internal/test/e2e/`
- **Swap Documentation**: See `docs/swaps.regtest.md`
- **Security Policy**: See `SECURITY.md` for vulnerability disclosure
