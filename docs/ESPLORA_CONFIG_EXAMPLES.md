# Esplora/Electrum Configuration Examples

# Example 1: Using Electrum Protocol with blockstream.info (Recommended for Mainnet)
FULMINE_ESPLORA_URL=blockstream.info:700

# Example 2: Using HTTP Esplora API with mempool.space (Backward Compatible)
# FULMINE_ESPLORA_URL=https://mempool.space/api

# Example 3: Using HTTP Esplora API with blockstream.info
# FULMINE_ESPLORA_URL=https://blockstream.info/api

# Example 4: Using Electrum Protocol with custom server
# FULMINE_ESPLORA_URL=custom-server.example.com:50002

# Example 5: Local regtest/testnet setup
# FULMINE_ESPLORA_URL=http://localhost:3000

# Notes:
# - Electrum servers are specified as host:port (no protocol prefix)
# - Common Electrum ports: 700 (blockstream TLS), 50002 (standard TLS), 50001 (plain TCP)
# - HTTP APIs are specified with http:// or https:// prefix
# - The service automatically detects the protocol and enables TLS for known secure ports
