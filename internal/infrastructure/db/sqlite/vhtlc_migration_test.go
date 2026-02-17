package sqlitedb_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/btcsuite/btcd/btcec/v2"
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
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'id'`).Scan(&hasID)
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
    FOREIGN KEY (vhtlc_id) REFERENCES vhtlc(preimage_hash) ON DELETE CASCADE
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

// setupNewVhtlcTable creates the new-schema vhtlc table (with id PRIMARY KEY and
// script column) for tests that need a post-migration schema.
func setupNewVhtlcTable(t *testing.T, dbh *sql.DB) {
	t.Helper()
	_, err := dbh.Exec(`
		CREATE TABLE IF NOT EXISTS vhtlc (
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
			unilateral_refund_without_receiver_delay_value INTEGER NOT NULL,
			script TEXT NOT NULL DEFAULT '[]'
		);
	`)
	require.NoError(t, err, "failed to create new-schema vhtlc table")
}

// setupNewVhtlcTableWithoutScript creates the new-schema vhtlc table WITHOUT the
// script column. Used to test the BackfillVhtlcScripts self-heal path.
func setupNewVhtlcTableWithoutScript(t *testing.T, dbh *sql.DB) {
	t.Helper()
	_, err := dbh.Exec(`
		CREATE TABLE IF NOT EXISTS vhtlc (
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
	`)
	require.NoError(t, err, "failed to create vhtlc table without script column")
}

// insertNewSchemaRowWithScript inserts a VHTLC row into the new-schema table
// (which has a script column). Uses real secp256k1 keys so BackfillVhtlcScripts
// can derive a valid taproot script for the row.
func insertNewSchemaRowWithoutScript(t *testing.T, dbh *sql.DB) (id string) {
	t.Helper()
	preimage := make([]byte, 20)
	for i := range preimage {
		preimage[i] = byte(i + 1)
	}

	senderKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	receiverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	serverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	sender := senderKey.PubKey().SerializeCompressed()
	receiver := receiverKey.PubKey().SerializeCompressed()
	server := serverKey.PubKey().SerializeCompressed()

	id = domain.GetVhtlcId(preimage, sender, receiver)

	_, err = dbh.Exec(`
		INSERT INTO vhtlc (
			id, preimage_hash, sender, receiver, server, refund_locktime,
			unilateral_claim_delay_type, unilateral_claim_delay_value,
			unilateral_refund_delay_type, unilateral_refund_delay_value,
			unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
		hex.EncodeToString(preimage),
		hex.EncodeToString(sender),
		hex.EncodeToString(receiver),
		hex.EncodeToString(server),
		500000, 1, 10, 1, 20, 1, 30,
	)
	require.NoError(t, err, "failed to insert test row")
	return id
}

// TestBackfillVhtlcIdempotent verifies that calling BackfillVhtlc on a DB that
// has already been migrated (id column exists) is a no-op: it returns nil and
// leaves the row count unchanged.
func TestBackfillVhtlcIdempotent(t *testing.T) {
	ctx := context.Background()
	dbh, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)
	defer dbh.Close()

	setupOldVhtlcTable(t, dbh)
	setupOldSwapTable(t, dbh)
	insertTestRows(t, dbh)

	// First call: migrates data.
	err = sqlitedb.BackfillVhtlc(ctx, dbh)
	require.NoError(t, err, "first BackfillVhtlc call must succeed")

	var firstCount int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM vhtlc`).Scan(&firstCount)
	require.NoError(t, err)

	// Second call: id column exists → early return, no-op.
	err = sqlitedb.BackfillVhtlc(ctx, dbh)
	require.NoError(t, err, "second BackfillVhtlc call must not return error")

	var secondCount int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM vhtlc`).Scan(&secondCount)
	require.NoError(t, err)
	require.Equal(t, firstCount, secondCount,
		"row count must be unchanged after idempotent BackfillVhtlc call")
}

// TestBackfillVhtlcScriptsIdempotent verifies that calling BackfillVhtlcScripts
// twice on a fully-backfilled DB returns nil and leaves the script values unchanged.
func TestBackfillVhtlcScriptsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbh, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)
	defer dbh.Close()

	setupNewVhtlcTable(t, dbh)
	id := insertNewSchemaRowWithoutScript(t, dbh)

	// First call: populates the script.
	err = sqlitedb.BackfillVhtlcScripts(ctx, dbh)
	require.NoError(t, err, "first BackfillVhtlcScripts call must succeed")

	var firstScript string
	err = dbh.QueryRow(`SELECT script FROM vhtlc WHERE id = ?`, id).Scan(&firstScript)
	require.NoError(t, err)
	require.NotEqual(t, "[]", firstScript,
		"script must be populated after first BackfillVhtlcScripts call")

	// Second call: all scripts already populated (WHERE script='[]' matches nothing).
	err = sqlitedb.BackfillVhtlcScripts(ctx, dbh)
	require.NoError(t, err, "second BackfillVhtlcScripts call must not return error")

	var secondScript string
	err = dbh.QueryRow(`SELECT script FROM vhtlc WHERE id = ?`, id).Scan(&secondScript)
	require.NoError(t, err)
	require.Equal(t, firstScript, secondScript,
		"script value must not change on idempotent BackfillVhtlcScripts call")
}

// TestBackfillVhtlcScriptsSelfHeal verifies that BackfillVhtlcScripts can add
// a missing script column (self-heal path) and populate it correctly.
// This simulates a DB that has the new id PRIMARY KEY schema but was seeded
// before the SQL migration for the script column was applied.
func TestBackfillVhtlcScriptsSelfHeal(t *testing.T) {
	ctx := context.Background()
	dbh, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)
	defer dbh.Close()

	// Create vhtlc table WITH new PK schema but WITHOUT script column.
	setupNewVhtlcTableWithoutScript(t, dbh)
	insertNewSchemaRowWithoutScript(t, dbh)

	// Verify script column is absent before calling BackfillVhtlcScripts.
	var scriptColCount int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'script'`).Scan(&scriptColCount)
	require.NoError(t, err)
	require.Equal(t, 0, scriptColCount, "script column must not exist before self-heal")

	// BackfillVhtlcScripts must add the missing column and populate it.
	err = sqlitedb.BackfillVhtlcScripts(ctx, dbh)
	require.NoError(t, err, "BackfillVhtlcScripts must succeed even when script column is absent")

	// Verify script column now exists.
	err = dbh.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'script'`).Scan(&scriptColCount)
	require.NoError(t, err)
	require.Equal(t, 1, scriptColCount, "script column must exist after self-heal")

	// Verify the script was populated (not '[]' sentinel).
	var script string
	err = dbh.QueryRow(`SELECT script FROM vhtlc LIMIT 1`).Scan(&script)
	require.NoError(t, err)
	require.NotEqual(t, "[]", script,
		"script must be populated (not '[]' sentinel) after self-heal BackfillVhtlcScripts")
	require.NotEmpty(t, script, "script must not be empty after self-heal")
}
