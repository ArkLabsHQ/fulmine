-- Rollback: Remove script column from vhtlc table.
-- SQLite does not support ALTER TABLE DROP COLUMN before version 3.35.0 (2021-03-12).
-- Use the canonical create/copy/drop/rename pattern instead.

CREATE TABLE IF NOT EXISTS vhtlc_rollback (
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

INSERT INTO vhtlc_rollback SELECT
    id, preimage_hash, sender, receiver, server, refund_locktime,
    unilateral_claim_delay_type, unilateral_claim_delay_value,
    unilateral_refund_delay_type, unilateral_refund_delay_value,
    unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
FROM vhtlc;

DROP TABLE vhtlc;
ALTER TABLE vhtlc_rollback RENAME TO vhtlc;

-- Remove the Go migration record so BackfillVhtlcScripts re-runs if this
-- migration is re-applied. Create the table defensively for old DBs where the
-- registry may not exist yet.
CREATE TABLE IF NOT EXISTS go_migrations (
    version TEXT PRIMARY KEY,
    applied_at INTEGER NOT NULL
);
DELETE FROM go_migrations WHERE version = '20260216162009';
