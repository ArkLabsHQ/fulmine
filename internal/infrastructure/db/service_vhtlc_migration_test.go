package db_test

import (
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// TestSQLiteStartupBackfillsVhtlcScripts verifies the end-to-end integration:
// a database seeded at a pre-script schema version (20250828142000) gets its
// VHTLC scripts populated when db.NewService runs all migrations including the
// 20260216162009 SQL migration and BackfillVhtlcScripts Go migration.
func TestSQLiteStartupBackfillsVhtlcScripts(t *testing.T) {
	dbDir := t.TempDir()
	dbFile := filepath.Join(dbDir, "fulmine.db")
	dbh, err := sqlitedb.OpenDb(dbFile)
	require.NoError(t, err)

	seedPreScriptSchema(t, dbh)
	seedVhtlcRowWithoutScript(t, dbh)
	require.NoError(t, dbh.Close())

	svc, err := db.NewService(db.ServiceConfig{
		DbType:   "sqlite",
		DbConfig: []any{dbDir},
	})
	require.NoError(t, err)
	svc.Close()

	dbh, err = sqlitedb.OpenDb(dbFile)
	require.NoError(t, err)
	defer dbh.Close()

	var script string
	err = dbh.QueryRow(`SELECT script FROM vhtlc LIMIT 1`).Scan(&script)
	require.NoError(t, err)
	require.NotEqual(t, "[]", script, "script must be populated by BackfillVhtlcScripts")
	require.NotEmpty(t, script)
	scriptBytes, err := hex.DecodeString(script)
	require.NoError(t, err)
	require.NotEmpty(t, scriptBytes)
}

// TestSQLiteStartupIdempotent verifies that calling db.NewService twice on
// the same database does not return an error and does not re-run migrations.
// After both calls, go_migrations must contain exactly 2 rows (one per registered
// Go migration).
func TestSQLiteStartupIdempotent(t *testing.T) {
	dbDir := t.TempDir()
	dbFile := filepath.Join(dbDir, "fulmine.db")

	// First startup.
	svc1, err := db.NewService(db.ServiceConfig{
		DbType:   "sqlite",
		DbConfig: []any{dbDir},
	})
	require.NoError(t, err, "first NewService call must succeed")
	svc1.Close()

	// Second startup on the same DB.
	svc2, err := db.NewService(db.ServiceConfig{
		DbType:   "sqlite",
		DbConfig: []any{dbDir},
	})
	require.NoError(t, err, "second NewService call on same DB must succeed without error")
	svc2.Close()

	// Verify go_migrations has exactly 2 rows (BackfillVhtlc and BackfillVhtlcScripts).
	dbh, err := sqlitedb.OpenDb(dbFile)
	require.NoError(t, err)
	defer dbh.Close()

	var count int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM go_migrations`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count,
		"go_migrations must have exactly 2 rows after two NewService calls (each migration recorded once)")
}

// TestSQLiteColdStart verifies that starting with a completely fresh directory
// (no pre-existing DB file) creates all expected tables and completes all
// migrations including go_migrations.
func TestSQLiteColdStart(t *testing.T) {
	dbDir := t.TempDir() // No pre-existing DB file.
	dbFile := filepath.Join(dbDir, "fulmine.db")

	svc, err := db.NewService(db.ServiceConfig{
		DbType:   "sqlite",
		DbConfig: []any{dbDir},
	})
	require.NoError(t, err, "cold start from empty directory must succeed")
	svc.Close()

	dbh, err := sqlitedb.OpenDb(dbFile)
	require.NoError(t, err)
	defer dbh.Close()

	// Verify schema_migrations table exists.
	var schemaMigrationCount int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&schemaMigrationCount)
	require.NoError(t, err, "schema_migrations table must exist after cold start")
	require.Greater(t, schemaMigrationCount, 0, "schema_migrations must have at least one row")

	// Verify go_migrations table exists and has 2 rows.
	var goMigrationCount int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM go_migrations`).Scan(&goMigrationCount)
	require.NoError(t, err, "go_migrations table must exist after cold start")
	require.Equal(t, 2, goMigrationCount,
		"go_migrations must have 2 rows (BackfillVhtlc and BackfillVhtlcScripts) after cold start")

	// Verify both expected versions are present.
	var v1Count, v2Count int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM go_migrations WHERE version = '20250622101533'`).Scan(&v1Count)
	require.NoError(t, err)
	require.Equal(t, 1, v1Count, "BackfillVhtlc (20250622101533) must be recorded")

	err = dbh.QueryRow(`SELECT COUNT(*) FROM go_migrations WHERE version = '20260216162009'`).Scan(&v2Count)
	require.NoError(t, err)
	require.Equal(t, 1, v2Count, "BackfillVhtlcScripts (20260216162009) must be recorded")

	// Verify vhtlc table exists with id TEXT PRIMARY KEY and script column.
	var hasID, hasScript int
	err = dbh.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'id'`).Scan(&hasID)
	require.NoError(t, err)
	require.Equal(t, 1, hasID, "vhtlc table must have id column after cold start")

	err = dbh.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc') WHERE name = 'script'`).Scan(&hasScript)
	require.NoError(t, err)
	require.Equal(t, 1, hasScript, "vhtlc table must have script column after cold start")
}

// seedPreScriptSchema creates the database tables as they would exist at
// schema_migrations version 20250828142000 (before the script column was added).
// This simulates an existing user database being upgraded.
func seedPreScriptSchema(t *testing.T, dbh *sql.DB) {
	_, err := dbh.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER NOT NULL PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		);
		INSERT INTO schema_migrations(version, dirty) VALUES (20250828142000, false);

		CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			api_root TEXT NOT NULL,
			server_url TEXT NOT NULL,
			esplora_url TEXT,
			currency TEXT NOT NULL,
			event_server TEXT NOT NULL,
			full_node TEXT NOT NULL,
			ln_url TEXT,
			unit TEXT NOT NULL,
			ln_datadir TEXT,
			ln_type INTEGER CHECK(ln_type IN(0,1, 2))
		);

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

		CREATE TABLE IF NOT EXISTS swap (
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
		);

		CREATE TABLE IF NOT EXISTS vtxo_rollover (
			address TEXT PRIMARY KEY,
			taproot_tree TEXT NOT NULL,
			destination_address TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS subscribed_script (
			script TEXT PRIMARY KEY
		);
	`)
	require.NoError(t, err)
}

// seedVhtlcRowWithoutScript inserts a valid VHTLC row (without a script column)
// into the vhtlc table. Caller must ensure the table exists without the script
// column. The row uses real secp256k1 compressed public keys so that
// BackfillVhtlcScripts can derive a valid taproot locking script for it.
func seedVhtlcRowWithoutScript(t *testing.T, dbh *sql.DB) {
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

	id := domain.GetVhtlcId(preimage, sender, receiver)

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
		500000,
		1,
		10,
		1,
		20,
		1,
		30,
	)
	require.NoError(t, err)
}
