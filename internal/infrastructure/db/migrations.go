package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// GoMigration is a named data migration that runs after the schema migration
// identified by Version has been applied. Run must be idempotent: if the
// process crashes after Run returns nil but before the completion row is
// written, Run will be called again on the next startup and must produce the
// same result.
//
// Version must match the golang-migrate version number of the corresponding
// SQL migration file (e.g., "20260216162009"). This creates a natural
// cross-reference: SELECT * FROM go_migrations shows exactly which SQL
// migration versions triggered Go-side data backfill work.
type GoMigration struct {
	Version string // matches the golang-migrate version number, stored as TEXT
	Run     func(ctx context.Context, db *sql.DB) error
}

// ApplyGoMigrations runs any GoMigration whose Version has not yet been
// recorded in the go_migrations table. Migrations are applied in slice order
// (chronological by version). If a migration's Run function returns an error,
// ApplyGoMigrations returns that error immediately without recording the
// migration as applied; the next startup will retry it.
//
// The go_migrations table is created by this function if it does not exist.
// It is NOT created by a SQL migration file; this keeps the registry fully
// self-contained. Note: go_migrations should be included in any DB backup
// and restore tooling — its loss causes Go migrations to re-run on next
// startup, which is safe (Run must be idempotent) but incurs extra work.
func ApplyGoMigrations(ctx context.Context, db *sql.DB, migrations []GoMigration) error {
	seenVersions := make(map[string]struct{}, len(migrations))
	for _, m := range migrations {
		if m.Version == "" {
			return fmt.Errorf("go migration has empty version")
		}
		if _, err := strconv.ParseInt(m.Version, 10, 64); err != nil {
			return fmt.Errorf("go migration %q has invalid version format: %w", m.Version, err)
		}
		if _, exists := seenVersions[m.Version]; exists {
			return fmt.Errorf("duplicate go migration version %s", m.Version)
		}
		seenVersions[m.Version] = struct{}{}
	}

	if _, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS go_migrations (
				version    TEXT PRIMARY KEY,
				applied_at INTEGER NOT NULL
			);
		`); err != nil {
		return fmt.Errorf("ensure go_migrations table: %w", err)
	}

	var schemaVersion int64
	if err := db.QueryRowContext(ctx, `SELECT version FROM schema_migrations LIMIT 1`).Scan(&schemaVersion); err != nil {
		return fmt.Errorf("read schema migration version: %w", err)
	}

	for _, m := range migrations {
		migrationVersion, _ := strconv.ParseInt(m.Version, 10, 64)
		if schemaVersion < migrationVersion {
			return fmt.Errorf(
				"go migration %s requires SQL migration %s, but schema version is %d",
				m.Version, m.Version, schemaVersion,
			)
		}

		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM go_migrations WHERE version = ?`, m.Version,
		).Scan(&count); err != nil {
			return fmt.Errorf("check go migration %s: %w", m.Version, err)
		}
		if count > 0 {
			continue // already applied on a previous startup
		}

		if err := m.Run(ctx, db); err != nil {
			return fmt.Errorf("go migration %s: %w", m.Version, err)
		}

		if _, err := db.ExecContext(ctx,
			`INSERT INTO go_migrations (version, applied_at) VALUES (?, ?)`,
			m.Version, time.Now().Unix(),
		); err != nil {
			return fmt.Errorf("record go migration %s: %w", m.Version, err)
		}
	}

	return nil
}
