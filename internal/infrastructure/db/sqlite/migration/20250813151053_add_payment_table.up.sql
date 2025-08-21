CREATE TABLE IF NOT EXISTS payment (
    id TEXT PRIMARY KEY,
    amount INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    payment_type INTEGER NOT NULL CHECK(payment_type IN(0,1)),
    status INTEGER NOT NULL CHECK(status IN(0,1,2,3)),
    invoice TEXT NOT NULL,
    tx_id TEXT NOT NULL,
    reclaim_tx_id TEXT,
    vhtlc_id TEXT NOT NULL,
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(preimage_hash) ON DELETE CASCADE
);