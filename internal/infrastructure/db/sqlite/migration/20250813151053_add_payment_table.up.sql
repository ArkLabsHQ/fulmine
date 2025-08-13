CREATE TABLE IF NOT EXISTS payment (
    id TEXT PRIMARY KEY,
    amount INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    payment_type INTEGER NOT NULL CHECK(payment_type IN(0,1)),
    status INTEGER NOT NULL CHECK(status IN(0,1,2)),
    invoice TEXT NOT NULL,
    tx_id TEXT NOT NULL
);