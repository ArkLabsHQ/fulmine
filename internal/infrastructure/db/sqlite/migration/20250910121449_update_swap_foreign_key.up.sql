-- 00XX_swap_fk_to_vhtlc_id.up.sql
BEGIN;
PRAGMA foreign_keys = OFF;

-- 1) Create the replacement table with FK pointing to vhtlc(id)
CREATE TABLE swap_new (
    id TEXT PRIMARY KEY,
    amount INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    to_currency TEXT NOT NULL,
    from_currency TEXT NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2)),
    invoice TEXT NOT NULL,
    funding_tx_id TEXT NOT NULL,
    redeem_tx_id TEXT NOT NULL,
    vhtlc_id TEXT NOT NULL,
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(id) ON DELETE CASCADE
);

INSERT INTO swap_new (
  id, amount, timestamp, to_currency, from_currency, status, invoice,
  funding_tx_id, redeem_tx_id, vhtlc_id
)
SELECT
  s.id, s.amount, s.timestamp, s.to_currency, s.from_currency, s.status, s.invoice,
  s.funding_tx_id, s.redeem_tx_id,
  v.id AS vhtlc_id
FROM swap AS s
JOIN vhtlc AS v
  ON v.preimage_hash = s.vhtlc_id;

-- (Optional) Sanity check: row counts must match (no orphans)
WITH old_ct(c) AS (SELECT COUNT(*) FROM swap),
     new_ct(c) AS (SELECT COUNT(*) FROM swap_new)
SELECT CASE WHEN (SELECT c FROM old_ct) = (SELECT c FROM new_ct)
            THEN 1
            ELSE RAISE(ABORT, 'Backfill mismatch: some swap.vhtlc_id had no matching vhtlc.preimage_hash')
       END;

-- 3) Swap tables
DROP TABLE swap;
ALTER TABLE swap_new RENAME TO swap;

PRAGMA foreign_keys = ON;
COMMIT;