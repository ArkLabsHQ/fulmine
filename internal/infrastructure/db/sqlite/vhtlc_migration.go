package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
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
	const createNew = `CREATE TABLE IF NOT EXISTS swap_new (
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

	const copyData = `
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
