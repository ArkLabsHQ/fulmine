CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    api_root TEXT NOT NULL,
    server_url TEXT NOT NULL,
    esplora_url TEXT,
    currency TEXT NOT NULL,
    event_server TEXT NOT NULL,
    full_node TEXT NOT NULL,
    ln_url TEXT,
    unit TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vhtlc (
    preimage_hash TEXT PRIMARY KEY,
    sender TEXT NOT NULL,
    receiver TEXT NOT NULL,
    server TEXT NOT NULL,
    refund_locktime INTEGER NOT NULL,
    unilateral_claim_delay_type INTEGER NOT NULL,
    unilateral_claim_delay_value INTEGER NOT NULL,
    unilateral_refund_delay_type INTEGER NOT NULL,
    unilateral_refund_delay_value INTEGER NOT NULL,
    unilateral_refund_without_receiver_delay_type INTEGER NOT NULL,
    unilateral_refund_without_receiver_delay_value INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS vtxo_rollover (
    address TEXT PRIMARY KEY,
    taproot_tree TEXT NOT NULL,
    destination_address TEXT NOT NULL
);
