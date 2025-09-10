package db

import (
	"context"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
)

func setupOldVhtlcTable(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
        CREATE TABLE vhtlc (
            preimage_hash TEXT,
            sender TEXT,
            receiver TEXT,
            server TEXT,
            refund_locktime INTEGER,
            unilateral_claim_delay_type INTEGER,
            unilateral_claim_delay_value INTEGER,
            unilateral_refund_delay_type INTEGER,
            unilateral_refund_delay_value INTEGER,
            unilateral_refund_without_receiver_delay_type INTEGER,
            unilateral_refund_without_receiver_delay_value INTEGER
        );
    `)
	if err != nil {
		t.Fatalf("failed to create old vhtlc table: %v", err)
	}
}

func insertTestVhtlcRows(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
        INSERT INTO vhtlc (
            preimage_hash, sender, receiver, server, refund_locktime,
            unilateral_claim_delay_type, unilateral_claim_delay_value,
            unilateral_refund_delay_type, unilateral_refund_delay_value,
            unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
        ) VALUES
            ('aabbcc', '1122', '3344', 'srv1', 100, 1, 10, 2, 20, 3, 30),
            ('ddeeff', '5566', '7788', 'srv2', 200, 4, 40, 5, 50, 6, 60)
    `)
	if err != nil {
		t.Fatalf("failed to insert test vhtlc rows: %v", err)
	}
}

func TestVhtlcMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sqlitedb.OpenDb(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	setupOldVhtlcTable(t, db)
	insertTestVhtlcRows(t, db)

	if err := BackfillVhtlc(ctx, db); err != nil {
		t.Fatalf("migrate vhtlc: %v", err)
	}
	var hasID int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'id'`).Scan(&hasID); err != nil {
		t.Fatalf("pragma_table_info id: %v", err)
	}
	if hasID != 1 {
		t.Fatalf("expected vhtlc.id column to exist")
	}

	type row struct {
		ID       string
		Preimage string
		Sender   string
		Receiver string
	}
	rows, err := db.Query(`
        SELECT id, preimage_hash, sender, receiver
        FROM vhtlc
        ORDER BY sender
    `)
	if err != nil {
		t.Fatalf("query final vhtlc: %v", err)
	}
	defer rows.Close()

	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Preimage, &r.Sender, &r.Receiver); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 rows in migrated vhtlc, got %d", len(got))
	}

	// Check each id == sha256(sender|receiver|server)
	for _, r := range got {
		decodedPreimage, err := hex.DecodeString(r.Preimage)
		if err != nil {
			t.Fatalf("decode preimage %s: %v", r.Preimage, err)
		}
		sender, err := hex.DecodeString(r.Sender)
		if err != nil {
			t.Fatalf("decode sender %s: %v", r.Sender, err)
		}
		receiver, err := hex.DecodeString(r.Receiver)
		if err != nil {
			t.Fatalf("decode receiver %s: %v", r.Receiver, err)
		}

		want := domain.CreateVhtlcId(decodedPreimage, sender, receiver)
		if r.ID != want {
			t.Fatalf("bad id for sender=%s receiver=%s: got %s want %s",
				r.Sender, r.Receiver, r.ID, want)
		}
	}

}
