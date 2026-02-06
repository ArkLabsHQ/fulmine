CREATE TABLE IF NOT EXISTS chain_swap (
    id TEXT PRIMARY KEY,
    from_currency TEXT NOT NULL,
    to_currency TEXT NOT NULL,
    amount INTEGER NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2,3,4,5,6,7,8)),
    user_lockup_tx_id TEXT,
    server_lockup_tx_id TEXT,
    claim_tx_id TEXT,
    claim_preimage TEXT NOT NULL,
    refund_tx_id TEXT,
    user_btc_lockup_address TEXT,
    error_message TEXT,
    boltz_create_response_json TEXT,
    created_at INTEGER DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_chain_swap_status ON chain_swap(status);
