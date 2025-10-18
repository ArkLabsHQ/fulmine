# Esplora/Electrum Configuration Examples

# Example 1: Using Electrum Protocol with blockstream.info (Recommended for Mainnet)
FULMINE_ELECTRUM_URL=blockstream.info:700

# Example 2: Using HTTP Esplora API with mempool.space
FULMINE_ESPLORA_URL=https://mempool.space/api

# Example 3: Using HTTP Esplora API with blockstream.info
FULMINE_ESPLORA_URL=https://blockstream.info/api

# Example 4: Using Electrum Protocol with custom server
FULMINE_ELECTRUM_URL=custom-server.example.com:50002

# Example 5: Local regtest/testnet setup with HTTP
FULMINE_ESPLORA_URL=http://localhost:3000

# Example 6: Both URLs (Electrum takes priority)
FULMINE_ESPLORA_URL=https://mempool.space/api
FULMINE_ELECTRUM_URL=blockstream.info:700

# Notes:
# - FULMINE_ELECTRUM_URL: Electrum servers specified as host:port (e.g., blockstream.info:700)
# - FULMINE_ESPLORA_URL: HTTP APIs specified with http:// or https:// prefix
# - If both are set, Electrum takes priority
# - Common Electrum ports: 700 (blockstream TLS), 50002 (standard TLS), 50001 (plain TCP)
# - The service automatically enables TLS for known secure ports (700, 50002)
