package db

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
)

func ensureVhtlcNew(ctx context.Context, db *sql.DB) error {
	var tableExists int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vhtlc_new'`,
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("check vhtlc_new existence: %w", err)
	}
	if tableExists > 0 {
		// Still ensure the index exists.
		if _, err := db.ExecContext(ctx, `
			CREATE UNIQUE INDEX IF NOT EXISTS vhtlc_new_preimage_hash_sender_receiver_key
			ON vhtlc_new(preimage_hash, sender, receiver);
		`); err != nil {
			return fmt.Errorf("ensure vhtlc_new index: %w", err)
		}
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// BEGIN; PRAGMA foreign_keys = OFF;
	if _, err = tx.ExecContext(ctx, `PRAGMA foreign_keys = OFF;`); err != nil {
		return fmt.Errorf("pragma off: %w", err)
	}

	// CREATE TABLE vhtlc_new
	if _, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS vhtlc_new (
			id TEXT PRIMARY KEY,
			preimage_hash TEXT UNIQUE,
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
	`); err != nil {
		return fmt.Errorf("create vhtlc_new: %w", err)
	}

	// CREATE UNIQUE INDEX IF NOT EXISTS
	if _, err = tx.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS vhtlc_new_preimage_hash_sender_receiver_key
		ON vhtlc_new(preimage_hash, sender, receiver);
	`); err != nil {
		return fmt.Errorf("create vhtlc_new index: %w", err)
	}

	// PRAGMA foreign_keys = ON;
	if _, err = tx.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("pragma on: %w", err)
	}

	// COMMIT;
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func swapVhtlcTables(ctx context.Context, db *sql.DB) error {
	// If vhtlc_new doesn't exist, assume we've already swapped (nothing to do).
	var newExists int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vhtlc_new'`,
	).Scan(&newExists); err != nil {
		return fmt.Errorf("check vhtlc_new existence: %w", err)
	}
	if newExists == 0 {
		return nil
	}

	// If old vhtlc doesn't exist but vhtlc_new does, we still want to rename.
	var oldExists int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vhtlc'`,
	).Scan(&oldExists); err != nil {
		return fmt.Errorf("check vhtlc existence: %w", err)
	}

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

	// Sanity guard: if vhtlc exists, verify row counts match.
	if oldExists > 0 {
		var oldCT, newCT int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM vhtlc`).Scan(&oldCT); err != nil {
			return fmt.Errorf("count vhtlc: %w", err)
		}
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM vhtlc_new`).Scan(&newCT); err != nil {
			return fmt.Errorf("count vhtlc_new: %w", err)
		}
		if oldCT != newCT {
			return fmt.Errorf("backfill mismatch: vhtlc=%d vhtlc_new=%d", oldCT, newCT)
		}

		// Drop old table.
		if _, err = tx.ExecContext(ctx, `DROP TABLE vhtlc;`); err != nil {
			return fmt.Errorf("drop vhtlc: %w", err)
		}
	}

	// Rename new -> old name.
	if _, err = tx.ExecContext(ctx, `ALTER TABLE vhtlc_new RENAME TO vhtlc;`); err != nil {
		return fmt.Errorf("rename vhtlc_new->vhtlc: %w", err)
	}

	// Recreate helpful uniqueness guard on final table name (idempotent).
	if _, err = tx.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS vhtlc_preimage_hash_sender_receiver_server_key
		ON vhtlc(preimage_hash, sender, receiver, server);
	`); err != nil {
		return fmt.Errorf("create unique index: %w", err)
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

func BackfillVhtlc(ctx context.Context, dbh *sql.DB) error {

	if err := ensureVhtlcNew(ctx, dbh); err != nil {
		return fmt.Errorf("failed to ensure vhtlc_new table: %s", err)
	}

	if err := backfillVhtlc(context.Background(), dbh); err != nil {
		return fmt.Errorf("failed to backfill vhtlc: %s", err)
	}

	if err := swapVhtlcTables(ctx, dbh); err != nil {
		return fmt.Errorf("failed to swap vhtlc tables: %s", err)
	}

	return nil
}

func backfillVhtlc(ctx context.Context, dbh *sql.DB) error {

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
	type row struct {
		preimageHash   string
		sender         string
		receiver       string
		server         string
		refundLock     int64
		ucdt, ucdv     int64
		urdt, urdv     int64
		urwrdt, urwrdv int64
	}

	rows, err := dbh.QueryContext(ctx, list_VHTLC) // SELECT ... FROM vhtlc
	if err != nil {
		return err
	}

	var buf []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.preimageHash, &r.sender, &r.receiver, &r.server,
			&r.refundLock, &r.ucdt, &r.ucdv, &r.urdt, &r.urdv, &r.urwrdt, &r.urwrdv); err != nil {
			_ = rows.Close()
			return err
		}
		buf = append(buf, r)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 2) Begin write tx AFTER closing the read cursor
	tx, err := dbh.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, r := range buf {
		// If your inputs are hex strings, decode them before computing the ID
		preimageBytes, err := hex.DecodeString(r.preimageHash)
		if err != nil {
			return err
		}
		senderBytes, err := hex.DecodeString(r.sender)
		if err != nil {
			return err
		}
		receiverBytes, err := hex.DecodeString(r.receiver)
		if err != nil {
			return err
		}

		// domain.CreateVhtlcId should deterministically derive your new id
		id := domain.CreateVhtlcId(preimageBytes, senderBytes, receiverBytes)

		// IMPORTANT: insert into vhtlc_new (not vhtlc) during backfill
		// Ensure you have a corresponding sqlc query: InsertVHTLCNew
		if _, err := tx.ExecContext(ctx, insertVHTLC,
			id,
			r.preimageHash,
			r.sender,
			r.receiver,
			r.server,
			r.refundLock,
			r.ucdt,
			r.ucdv,
			r.urdt,
			r.urdv,
			r.urwrdt,
			r.urwrdv,
		); err != nil {
			return err
		}
	}

	// Commit
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil

}
