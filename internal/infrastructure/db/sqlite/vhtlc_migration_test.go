package sqlitedb_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/stretchr/testify/require"
)

func TestVhtlcMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)

	defer db.Close()

	setupOldVhtlcTable(t, db)
	setupOldSwapTable(t, db)
	insertTestRows(t, db)

	err = sqlitedb.BackfillVhtlc(ctx, db)
	require.NoError(t, err)

	var hasID int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'id'`).Scan(&hasID)
	require.NoError(t, err)
	require.Equal(t, 1, hasID)

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
	require.NoError(t, err)

	var got []row
	for rows.Next() {
		var r row
		err = rows.Scan(&r.ID, &r.Preimage, &r.Sender, &r.Receiver)
		require.NoError(t, err)
		got = append(got, r)
	}

	require.NoError(t, rows.Err())
	require.Len(t, got, 2)

	rows.Close()

	// Check each id == sha256(sender|receiver|server)
	for _, r := range got {
		decodedPreimage, err := hex.DecodeString(r.Preimage)
		require.NoError(t, err)

		sender, err := hex.DecodeString(r.Sender)
		require.NoError(t, err)

		receiver, err := hex.DecodeString(r.Receiver)
		require.NoError(t, err)

		want := domain.GetVhtlcId(decodedPreimage, sender, receiver)
		require.Equal(t, want, r.ID)

		swapRow := db.QueryRow(`SELECT vhtlc_id FROM swap WHERE vhtlc_id = ?`, r.ID)
		var swapId string
		err = swapRow.Scan(&swapId)
		require.NoError(t, err)
		require.Equal(t, r.ID, swapId)

	}

}

func setupOldVhtlcTable(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
        CREATE TABLE vhtlc (
            preimage_hash TEXT PRIMARY KEY,
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
	require.NoError(t, err, "failed to create old vhtlc table")
}

func setupOldSwapTable(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
	CREATE TABLE swap (
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
	`)

	require.NoError(t, err, "failed to create old swap table")
}

func insertTestRows(t *testing.T, db *sql.DB) {
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
	require.NoError(t, err, "failed to insert test vhtlc rows")

	_, err = db.Exec(`
		INSERT INTO swap (
			id, amount, timestamp, to_currency, from_currency, status, invoice,
			funding_tx_id, redeem_tx_id, vhtlc_id
		) VALUES
			('swap1', 1000, 1620000000, 'ARK', 'BTC', 0, 'inv1', 'fund1', 'redeem1', 'aabbcc'),
			('swap2', 2000, 1620003600, 'BTC', 'ARK', 1, 'inv2', 'fund2', 'redeem2', 'ddeeff')
	`)

	require.NoError(t, err, "failed to insert test swap rows")
}
