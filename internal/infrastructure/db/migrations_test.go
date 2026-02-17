package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// openTestDB opens an in-memory SQLite database for isolated unit testing.
// The returned *sql.DB is closed automatically when the test completes.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbh, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err, "open in-memory SQLite")
	t.Cleanup(func() { _ = dbh.Close() })
	_, err = dbh.Exec(`PRAGMA foreign_keys = ON`)
	require.NoError(t, err)

	_, err = dbh.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER NOT NULL PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		);
		INSERT INTO schema_migrations(version, dirty) VALUES (99999999999999, false);
	`)
	require.NoError(t, err)
	return dbh
}

// TestApplyGoMigrationsSkipsAlreadyApplied verifies that a migration whose
// version is already recorded in go_migrations is not called again.
func TestApplyGoMigrationsSkipsAlreadyApplied(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	// Pre-create the table and insert the version.
	_, err := dbh.ExecContext(ctx, `
		CREATE TABLE go_migrations (
			version    TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		);
			INSERT INTO go_migrations (version, applied_at) VALUES ('20250101000000', 0);
		`)
	require.NoError(t, err)

	called := false
	migrations := []db.GoMigration{
		{
			Version: "20250101000000",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				called = true
				return nil
			},
		},
	}

	err = db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err, "ApplyGoMigrations should not error when migration is already applied")
	require.False(t, called, "Run must not be called for an already-applied migration")
}

// TestApplyGoMigrationsRunsInOrder verifies that multiple pending migrations
// are applied in slice order, not in any other order.
func TestApplyGoMigrationsRunsInOrder(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	var executionOrder []string

	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				executionOrder = append(executionOrder, "first")
				return nil
			},
		},
		{
			Version: "20250101000002",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				executionOrder = append(executionOrder, "second")
				return nil
			},
		},
		{
			Version: "20250101000003",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				executionOrder = append(executionOrder, "third")
				return nil
			},
		},
	}

	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "third"}, executionOrder,
		"migrations must execute in slice order")
}

// TestApplyGoMigrationsFailFast verifies that when a migration's Run function
// returns an error, the dispatcher stops immediately without running subsequent
// migrations.
func TestApplyGoMigrationsFailFast(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	secondCalled := false
	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				return errors.New("intentional failure in first migration")
			},
		},
		{
			Version: "20250101000002",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				secondCalled = true
				return nil
			},
		},
	}

	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.Error(t, err, "ApplyGoMigrations must return error when a migration fails")
	require.ErrorContains(t, err, "intentional failure in first migration")
	require.False(t, secondCalled, "subsequent migration must not run after a failure")
}

// TestApplyGoMigrationsRecordsCompletion verifies that a successfully applied
// migration is recorded in go_migrations.
func TestApplyGoMigrationsRecordsCompletion(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				return nil
			},
		},
	}

	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err)

	var count int
	err = dbh.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM go_migrations WHERE version = '20250101000001'`,
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "completed migration must be recorded in go_migrations")
}

// TestApplyGoMigrationsPartialRetry verifies that a failed migration (no
// record written) is retried on the next call to ApplyGoMigrations.
func TestApplyGoMigrationsPartialRetry(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	attempt := 0
	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				attempt++
				if attempt == 1 {
					return errors.New("first attempt fails")
				}
				return nil
			},
		},
	}

	// First call: migration fails, no record written.
	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.Error(t, err, "first call must return error")
	require.Equal(t, 1, attempt, "Run was called once on first call")

	var count int
	err = dbh.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM go_migrations WHERE version = '20250101000001'`,
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "failed migration must NOT be recorded")

	// Second call: migration succeeds, record written.
	err = db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err, "second call must succeed")
	require.Equal(t, 2, attempt, "Run was called again on second call")

	err = dbh.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM go_migrations WHERE version = '20250101000001'`,
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "successfully retried migration must be recorded")
}

// TestApplyGoMigrationsIdempotent verifies that calling ApplyGoMigrations
// twice on the same DB does not re-run already-completed migrations.
func TestApplyGoMigrationsIdempotent(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	callCounts := map[string]int{}
	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				callCounts["m1"]++
				return nil
			},
		},
		{
			Version: "20250101000002",
			Run: func(ctx context.Context, dbh *sql.DB) error {
				callCounts["m2"]++
				return nil
			},
		},
	}

	// First call: both migrations run.
	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err)
	require.Equal(t, 1, callCounts["m1"], "m1 must run exactly once on first call")
	require.Equal(t, 1, callCounts["m2"], "m2 must run exactly once on first call")

	// Second call: both migrations are already recorded, neither runs again.
	err = db.ApplyGoMigrations(ctx, dbh, migrations)
	require.NoError(t, err)
	require.Equal(t, 1, callCounts["m1"], "m1 must NOT run again on second call")
	require.Equal(t, 1, callCounts["m2"], "m2 must NOT run again on second call")
}

func TestApplyGoMigrationsRejectsDuplicateVersions(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	migrations := []db.GoMigration{
		{Version: "20250101000001", Run: func(context.Context, *sql.DB) error { return nil }},
		{Version: "20250101000001", Run: func(context.Context, *sql.DB) error { return nil }},
	}

	err := db.ApplyGoMigrations(ctx, dbh, migrations)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate go migration version")
}

func TestApplyGoMigrationsRequiresSchemaVersion(t *testing.T) {
	dbh := openTestDB(t)
	ctx := context.Background()

	_, err := dbh.ExecContext(ctx, `DELETE FROM schema_migrations`)
	require.NoError(t, err)
	_, err = dbh.ExecContext(ctx, `INSERT INTO schema_migrations(version, dirty) VALUES (20250101000000, false)`)
	require.NoError(t, err)

	migrations := []db.GoMigration{
		{
			Version: "20250101000001",
			Run:     func(context.Context, *sql.DB) error { return nil },
		},
	}

	err = db.ApplyGoMigrations(ctx, dbh, migrations)
	require.Error(t, err)
	require.ErrorContains(t, err, "requires SQL migration")
}
