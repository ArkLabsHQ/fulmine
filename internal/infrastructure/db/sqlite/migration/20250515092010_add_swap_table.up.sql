CREATE TABLE IF NOT EXISTS swaps (
    id          TEXT    PRIMARY KEY,
    amount      INTEGER NOT NULL,
    date        TEXT    NOT NULL,
    "to"        TEXT    NOT NULL,
    "from"      TEXT    NOT NULL,
    is_pending  INTEGER NOT NULL DEFAULT 0,  -- 0 = false, 1 = true
    invoice     TEXT NOT NULL,
    vhltc_id    TEXT NOT NULL
);