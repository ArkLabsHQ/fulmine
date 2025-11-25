CREATE TABLE vhtlc (
    id TEXT PRIMARY KEY,
    preimage_hash TEXT UNIQUE NOT NULL,
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

CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    api_root TEXT NOT NULL,
    server_url TEXT NOT NULL,
    esplora_url TEXT,
    currency TEXT NOT NULL,
    event_server TEXT NOT NULL,
    full_node TEXT NOT NULL,
    ln_url TEXT,
    unit TEXT NOT NULL,
    ln_datadir TEXT,
    ln_type INTEGER CHECK(ln_type IN(0,1, 2))
);

CREATE TABLE IF NOT EXISTS swap (
    id TEXT PRIMARY KEY,
    amount INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    to_currency TEXT NOT NULL,
    from_currency TEXT NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2)),
    swap_type INTEGER CHECK(swap_type IN(0,1)) NOT NULL,
    invoice TEXT NOT NULL,
    funding_tx_id TEXT NOT NULL,
    redeem_tx_id TEXT NOT NULL,
    vhtlc_id TEXT NOT NULL,
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS vtxo_rollover (
    address TEXT PRIMARY KEY,
    taproot_tree TEXT NOT NULL,
    destination_address TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subscribed_script (
    script TEXT PRIMARY KEY
);