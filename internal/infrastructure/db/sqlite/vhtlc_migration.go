package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	log "github.com/sirupsen/logrus"
)

func BackfillVhtlc(ctx context.Context, dbh *sql.DB) error {
	var tableExists int

	if err := dbh.QueryRowContext(ctx,
		existsQuery("vhtlc", "id"),
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("failed to verify updated vhtlc existence: %w", err)
	}
	if tableExists > 0 {
		return nil
	}

	if err := ensureVhtlcNew(ctx, dbh); err != nil {
		return fmt.Errorf("failed to ensure vhtlc_new table: %s", err)
	}

	if err := backfillVhtlc(context.Background(), dbh); err != nil {
		return fmt.Errorf("failed to backfill vhtlc: %s", err)
	}

	if err := swapVhtlcTables(ctx, dbh); err != nil {
		return fmt.Errorf("failed to swap vhtlc tables: %s", err)
	}

	if err := fixSwapTableFK(ctx, dbh); err != nil {
		return fmt.Errorf("failed to fix swap table foreign keys: %s", err)
	}

	return nil
}

func ensureVhtlcNew(ctx context.Context, db *sql.DB) error {
	createVhtlc := `
		CREATE TABLE IF NOT EXISTS vhtlc_new (
			id TEXT PRIMARY KEY,
			preimage_hash TEXT NOT NULL,
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
	`

	if _, err := db.ExecContext(ctx, createVhtlc); err != nil {
		return fmt.Errorf("create vhtlc_new: %w", err)
	}

	return nil
}

func backfillVhtlc(ctx context.Context, db *sql.DB) error {

	list_VHTLC := `-- name: ListVHTLC :many
SELECT preimage_hash, sender, receiver, server, refund_locktime, unilateral_claim_delay_type, unilateral_claim_delay_value, unilateral_refund_delay_type, unilateral_refund_delay_value, unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value FROM vhtlc
`
	insertVHTLC := `-- name: InsertVHTLC :exec
INSERT INTO vhtlc_new (
    id, preimage_hash, sender, receiver, server, refund_locktime,
    unilateral_claim_delay_type, unilateral_claim_delay_value,
    unilateral_refund_delay_type, unilateral_refund_delay_value,
    unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, list_VHTLC) // SELECT ... FROM vhtlc
	if err != nil {
		return err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, insertVHTLC)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var (
			preimageHash, sender, receiver, server string
			refundLock                             int64
			ucdt, ucdv                             int64
			urdt, urdv                             int64
			urwrdt, urwrdv                         int64
		)
		if err = rows.Scan(&preimageHash, &sender, &receiver, &server,
			&refundLock, &ucdt, &ucdv, &urdt, &urdv, &urwrdt, &urwrdv); err != nil {
			return err
		}

		preB, err := hex.DecodeString(preimageHash)
		if err != nil {
			return err
		}
		sndB, err := hex.DecodeString(sender)
		if err != nil {
			return err
		}
		rcvB, err := hex.DecodeString(receiver)
		if err != nil {
			return err
		}

		id := domain.GetVhtlcId(preB, sndB, rcvB)

		log.Debug(fmt.Sprintf("vhtlc %s migrated -> id %s", preimageHash, id))

		if _, err = stmt.ExecContext(ctx,
			id, preimageHash, sender, receiver, server, refundLock,
			ucdt, ucdv, urdt, urdv, urwrdt, urwrdv,
		); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	err = tx.Commit()
	return err

}

func swapVhtlcTables(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Disable FKs for the swap window.
	if _, err = tx.ExecContext(ctx, `PRAGMA foreign_keys = OFF;`); err != nil {
		return fmt.Errorf("pragma off: %w", err)
	}

	var oldCT, newCT int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM vhtlc;`).Scan(&oldCT); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM vhtlc_new;`).Scan(&newCT); err != nil {
		return err
	}
	if oldCT != newCT {
		return fmt.Errorf("backfill mismatch: vhtlc=%d vhtlc_new=%d", oldCT, newCT)
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE vhtlc;`); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `ALTER TABLE vhtlc_new RENAME TO vhtlc;`); err != nil {
		return err
	}

	// Re-enable FKs.
	if _, err = tx.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("pragma on: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func fixSwapTableFK(ctx context.Context, db *sql.DB) error {
	// Check whether the swap table already has the swap_type column.
	// Migration 20250828142000 adds swap_type; if ALL SQL migrations ran before
	// BackfillVhtlc (as happens with the go_migrations registry), swap_type may
	// already exist and must be preserved during the table-rebuild.
	var swapTypeExists int
	if err := db.QueryRowContext(ctx,
		existsQuery("swap", "swap_type"),
	).Scan(&swapTypeExists); err != nil {
		return fmt.Errorf("check swap_type column: %w", err)
	}

	var createNew, copyData string
	if swapTypeExists > 0 {
		createNew = `CREATE TABLE IF NOT EXISTS swap_new (
    id TEXT PRIMARY KEY,
    amount INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    to_currency TEXT NOT NULL,
    from_currency TEXT NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2)),
    swap_type INTEGER CHECK(swap_type IN(0,1)) NOT NULL DEFAULT 0,
    invoice TEXT NOT NULL,
    funding_tx_id TEXT NOT NULL,
    redeem_tx_id TEXT NOT NULL,
    vhtlc_id TEXT NOT NULL,
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(id) ON DELETE CASCADE
	);`
		copyData = `
	INSERT INTO swap_new (
	  id, amount, timestamp, to_currency, from_currency, status, swap_type, invoice,
	  funding_tx_id, redeem_tx_id, vhtlc_id
	)
	SELECT
	  s.id, s.amount, s.timestamp, s.to_currency, s.from_currency, s.status, s.swap_type, s.invoice,
	  s.funding_tx_id, s.redeem_tx_id,
	  v.id AS vhtlc_id
	FROM swap AS s
	JOIN vhtlc AS v
	  ON v.preimage_hash = s.vhtlc_id;`
	} else {
		createNew = `CREATE TABLE IF NOT EXISTS swap_new (
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
	);`
		copyData = `
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
	  ON v.preimage_hash = s.vhtlc_id;`
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `PRAGMA foreign_keys = OFF;`); err != nil {
		return fmt.Errorf("disable fks: %w", err)
	}

	if _, err := tx.ExecContext(ctx, createNew); err != nil {
		return fmt.Errorf("create swap_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, copyData); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE swap;`); err != nil {
		return fmt.Errorf("drop old swap: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE swap_new RENAME TO swap;`); err != nil {
		return fmt.Errorf("rename new->swap: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable fks: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil

}

func existsQuery(tableName, columnName string) string {
	return fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = '%s'`, tableName, columnName)
}

// BackfillVhtlcScripts adds the script column to existing VHTLCs by
// regenerating taproot locking scripts from the stored VHTLC options.
//
// Pre-condition (belt-and-suspenders): the script column should already exist
// when this is called through the go_migrations registry, because the
// 20260216162009 SQL migration runs first. However, this function will
// self-heal DBs that previously ran a placeholder migration with no schema
// change by adding the missing column before populating it.
//
// The inner backfillVhtlcScripts uses WHERE script = '[]' OR script = ”,
// making it idempotent: already-populated rows are not re-processed.
func BackfillVhtlcScripts(ctx context.Context, dbh *sql.DB) error {
	var scriptColumnExists int
	if err := dbh.QueryRowContext(ctx,
		existsQuery("vhtlc", "script"),
	).Scan(&scriptColumnExists); err != nil {
		return fmt.Errorf("failed to verify script column existence: %w", err)
	}

	// Self-heal DBs that previously applied a placeholder migration with no
	// schema change. The go_migrations registry normally prevents this, but
	// the guard is kept for belt-and-suspenders safety.
	if scriptColumnExists == 0 {
		if _, err := dbh.ExecContext(ctx, `ALTER TABLE vhtlc ADD COLUMN script TEXT NOT NULL DEFAULT '[]';`); err != nil {
			return fmt.Errorf("failed to add missing script column: %w", err)
		}
	}

	if err := backfillVhtlcScripts(ctx, dbh); err != nil {
		return fmt.Errorf("failed to backfill vhtlc scripts: %w", err)
	}

	log.Info("vhtlc script column backfill completed successfully")
	return nil
}

func backfillVhtlcScripts(ctx context.Context, db *sql.DB) error {
	listVHTLC := `
SELECT id, preimage_hash, sender, receiver, server, refund_locktime,
       unilateral_claim_delay_type, unilateral_claim_delay_value,
       unilateral_refund_delay_type, unilateral_refund_delay_value,
       unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
FROM vhtlc
WHERE script = '[]' OR script = ''
	`
	updateVHTLC := `
UPDATE vhtlc
SET script = ?
WHERE id = ?
	`

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, listVHTLC)
	if err != nil {
		return fmt.Errorf("query existing vhtlcs: %w", err)
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, updateVHTLC)
	if err != nil {
		return fmt.Errorf("prepare update statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var (
			id, preimageHash, sender, receiver, server string
			refundLocktime                             int64
			ucdt, ucdv                                 int64
			urdt, urdv                                 int64
			urwrdt, urwrdv                             int64
		)

		if err = rows.Scan(&id, &preimageHash, &sender, &receiver, &server,
			&refundLocktime, &ucdt, &ucdv, &urdt, &urdv, &urwrdt, &urwrdv); err != nil {
			return fmt.Errorf("scan vhtlc row: %w", err)
		}

		// Decode pubkeys to regenerate scripts.
		preimageHashBytes, err := hex.DecodeString(preimageHash)
		if err != nil {
			return fmt.Errorf("decode preimage hash for %s: %w", id, err)
		}
		senderBytes, err := hex.DecodeString(sender)
		if err != nil {
			return fmt.Errorf("decode sender for %s: %w", id, err)
		}
		receiverBytes, err := hex.DecodeString(receiver)
		if err != nil {
			return fmt.Errorf("decode receiver for %s: %w", id, err)
		}
		serverBytes, err := hex.DecodeString(server)
		if err != nil {
			return fmt.Errorf("decode server for %s: %w", id, err)
		}

		senderPubkey, err := btcec.ParsePubKey(senderBytes)
		if err != nil {
			return fmt.Errorf("parse sender pubkey for %s: %w", id, err)
		}
		receiverPubkey, err := btcec.ParsePubKey(receiverBytes)
		if err != nil {
			return fmt.Errorf("parse receiver pubkey for %s: %w", id, err)
		}
		serverPubkey, err := btcec.ParsePubKey(serverBytes)
		if err != nil {
			return fmt.Errorf("parse server pubkey for %s: %w", id, err)
		}

		opts := vhtlc.Opts{
			PreimageHash:   preimageHashBytes,
			Sender:         senderPubkey,
			Receiver:       receiverPubkey,
			Server:         serverPubkey,
			RefundLocktime: arklib.AbsoluteLocktime(refundLocktime),
			UnilateralClaimDelay: arklib.RelativeLocktime{
				Type:  arklib.RelativeLocktimeType(ucdt),
				Value: uint32(ucdv),
			},
			UnilateralRefundDelay: arklib.RelativeLocktime{
				Type:  arklib.RelativeLocktimeType(urdt),
				Value: uint32(urdv),
			},
			UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
				Type:  arklib.RelativeLocktimeType(urwrdt),
				Value: uint32(urwrdv),
			},
		}

		lockingScriptHex, err := vhtlc.LockingScriptHexFromOpts(opts)
		if err != nil {
			log.WithError(err).Warnf("failed to derive locking script for vhtlc %s, using empty array", id)
			if _, err = stmt.ExecContext(ctx, "[]", id); err != nil {
				return fmt.Errorf("update vhtlc %s with empty script: %w", id, err)
			}
			count++
			continue
		}

		if _, err = stmt.ExecContext(ctx, lockingScriptHex, id); err != nil {
			return fmt.Errorf("update vhtlc %s: %w", id, err)
		}

		count++
		if count%100 == 0 {
			log.Debugf("backfilled %d vhtlcs with scripts...", count)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	log.Infof("backfilled %d vhtlcs with scripts", count)

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
