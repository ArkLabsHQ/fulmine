-- Add script column. Default '[]' is a safe sentinel; actual population
-- is handled by the Go migration registered under this same version
-- (see goMigrations in internal/infrastructure/db/service.go).
ALTER TABLE vhtlc ADD COLUMN script TEXT NOT NULL DEFAULT '[]';
