DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS vtxo_rollover;
DROP TABLE IF EXISTS swap;
DROP TABLE IF EXISTS chain_swap;
DROP INDEX IF EXISTS idx_chain_swap_status;

ALTER TABLE vhtlc RENAME TO vhtlc_legacy;

CREATE TABLE vhtlc (
    id TEXT PRIMARY KEY,
    script TEXT NOT NULL
);
