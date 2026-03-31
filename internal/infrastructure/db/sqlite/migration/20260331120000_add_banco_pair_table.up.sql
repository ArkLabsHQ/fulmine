CREATE TABLE IF NOT EXISTS banco_pair (
    pair TEXT PRIMARY KEY,
    quote_asset_id TEXT NOT NULL DEFAULT '',
    min_amount INTEGER NOT NULL,
    max_amount INTEGER NOT NULL,
    price_feed TEXT NOT NULL
);
