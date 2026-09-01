package sqlitedb_test

import (
	"context"
	"testing"

	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/stretchr/testify/require"
)

func TestLegacyVhtlcRepo(t *testing.T) {
	ctx := context.Background()
	db, err := sqlitedb.OpenDb(":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE vhtlc_legacy (
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
	_, err = db.Exec(
		`INSERT INTO vhtlc_legacy VALUES ('id1','aa','bb','cc','dd',100,1,10,1,20,0,30);`,
	)
	require.NoError(t, err)

	repo, err := sqlitedb.NewVHTLCRepository(db)
	require.NoError(t, err)

	has, err := repo.HasLegacy(ctx)
	require.NoError(t, err)
	require.True(t, has)

	rows, err := repo.GetLegacy(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "bb", rows[0].Sender)
	require.Equal(t, "cc", rows[0].Receiver)
	require.Equal(t, int64(100), rows[0].RefundLocktime)
	require.Equal(t, int64(30), rows[0].UnilateralRefundWithoutReceiverDelayValue)

	require.NoError(t, repo.DropLegacy(ctx))
	has, err = repo.HasLegacy(ctx)
	require.NoError(t, err)
	require.False(t, has)
}
