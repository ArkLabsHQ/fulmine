-- 00XX_swap_fk_to_vhtlc_id.down.sql
BEGIN;
PRAGMA foreign_keys = OFF;

-- 1) Recreate the previous schema: FK -> vhtlc(preimage_hash)
CREATE TABLE swap_prev (
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
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(preimage_hash) ON DELETE CASCADE
);

-- (Recreate any indexes you had on swap; examples:)
-- CREATE INDEX IF NOT EXISTS swap_status_idx ON swap_prev(status);
-- CREATE INDEX IF NOT EXISTS swap_vhtlc_id_idx ON swap_prev(vhtlc_id);

-- 2) Copy rows while converting vhtlc.id -> vhtlc.preimage_hash
INSERT INTO swap_prev (
  id, amount, timestamp, to_currency, from_currency, status, invoice,
  funding_tx_id, redeem_tx_id, vhtlc_id
)
SELECT
  s.id, s.amount, s.timestamp, s.to_currency, s.from_currency, s.status, s.invoice,
  s.funding_tx_id, s.redeem_tx_id,
  v.preimage_hash AS vhtlc_id
FROM swap AS s
JOIN vhtlc AS v
  ON v.id = s.vhtlc_id;

-- Optional: sanity check (abort on mismatch)
WITH old_ct(c) AS (SELECT COUNT(*) FROM swap),
     new_ct(c) AS (SELECT COUNT(*) FROM swap_prev)
SELECT CASE WHEN (SELECT c FROM old_ct) = (SELECT c FROM new_ct)
            THEN 1
            ELSE RAISE(ABORT, 'Down-migration mismatch: some swap.vhtlc_id had no matching vhtlc.id')
       END;

-- 3) Swap tables back
DROP TABLE swap;
ALTER TABLE swap_prev RENAME TO swap;

PRAGMA foreign_keys = ON;
COMMIT;