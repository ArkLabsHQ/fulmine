CREATE TABLE IF NOT EXISTS swaps (
    id          TEXT    PRIMARY KEY,
    amount      INTEGER NOT NULL,
    date        TEXT    NOT NULL,
    "to"        TEXT    NOT NULL,
    "from"      TEXT    NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2)),
    invoice     TEXT NOT NULL,
    vhltc_id    TEXT NOT NULL
);