package sqlitedb_test

import (
	"os"
	"testing"

	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/stretchr/testify/require"
)

func TestCleanupMigrationRenamesVhtlc(t *testing.T) {
	db, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Old parameter-rich vhtlc table with one row.
	_, err = db.Exec(`
		CREATE TABLE vhtlc (
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
		);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO vhtlc VALUES ('id1','aa','bb','cc','dd',100,1,10,1,20,0,30);`)
	require.NoError(t, err)

	up, err := os.ReadFile("migration/20260720000000_cleanup.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err)

	// Legacy table keeps the row and its columns.
	var legacyCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vhtlc_legacy`).Scan(&legacyCount))
	require.Equal(t, 1, legacyCount)
	var hasPreimage int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('vhtlc_legacy') WHERE name = 'preimage_hash'`,
	).Scan(&hasPreimage))
	require.Equal(t, 1, hasPreimage)

	// New vhtlc table exists with exactly id + script and no rows.
	var cols int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vhtlc')`).Scan(&cols))
	require.Equal(t, 2, cols)
	var newCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vhtlc`).Scan(&newCount))
	require.Equal(t, 0, newCount)
}
