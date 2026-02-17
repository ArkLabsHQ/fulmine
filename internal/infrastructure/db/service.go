package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	badgerdb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/badger"
	sqlitedb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite"
	"github.com/dgraph-io/badger/v4"
	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const (
	sqliteDbFile = "fulmine.db"
)

var (
	//go:embed sqlite/migration/*
	migrations   embed.FS
	allowedTypes = strings.Join([]string{"badger"}, ",")
)

// goMigrations lists all Go-side data migrations in chronological order.
// Each entry's Version matches the golang-migrate version of the SQL migration
// that introduces the corresponding schema change.
var goMigrations = []GoMigration{
	{
		Version: "20250622101533",
		Run: func(ctx context.Context, db *sql.DB) error {
			return sqlitedb.BackfillVhtlc(ctx, db)
		},
	},
	{
		Version: "20260216162009",
		Run: func(ctx context.Context, db *sql.DB) error {
			return sqlitedb.BackfillVhtlcScripts(ctx, db)
		},
	},
}

type ServiceConfig struct {
	DbType   string
	DbConfig []any
}

type service struct {
	settingsRepo         domain.SettingsRepository
	vhtlcRepo            domain.VHTLCRepository
	vtxoRolloverRepo     domain.VtxoRolloverRepository
	swapRepo             domain.SwapRepository
	subscribedScriptRepo domain.SubscribedScriptRepository
}

func NewService(config ServiceConfig) (ports.RepoManager, error) {
	var (
		settingsRepo         domain.SettingsRepository
		vhtlcRepo            domain.VHTLCRepository
		vtxoRolloverRepo     domain.VtxoRolloverRepository
		swapRepo             domain.SwapRepository
		subscribedScriptRepo domain.SubscribedScriptRepository
		err                  error
	)

	switch config.DbType {
	case "badger":
		if len(config.DbConfig) != 2 {
			return nil, fmt.Errorf("badger db config must have 2 elements, got %d", len(config.DbConfig))
		}
		baseDir, ok := config.DbConfig[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid base directory")
		}
		var logger badger.Logger
		if config.DbConfig[1] != nil {
			logger, ok = config.DbConfig[1].(badger.Logger)
			if !ok {
				return nil, fmt.Errorf("invalid logger")
			}
		}
		settingsRepo, err = badgerdb.NewSettingsRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open settings db: %s", err)
		}
		vhtlcRepo, err = badgerdb.NewVHTLCRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open vhtlc db: %s", err)
		}
		vtxoRolloverRepo, err = badgerdb.NewVtxoRolloverRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open vtxo rollover db: %s", err)
		}
		swapRepo, err = badgerdb.NewSwapRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open swap db: %s", err)
		}

		subscribedScriptRepo, err = badgerdb.NewSubscribedScriptRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open subscribed script db: %s", err)
		}

	case "sqlite":
		if len(config.DbConfig) != 1 {
			return nil, fmt.Errorf("sqlite db config must have 1 element, got %d", len(config.DbConfig))
		}
		baseDir, ok := config.DbConfig[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid base directory")
		}
		dbFile := filepath.Join(baseDir, sqliteDbFile)
		db, err := sqlitedb.OpenDb(dbFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite db: %s", err)
		}

		driver, err := sqlitemigrate.WithInstance(db, &sqlitemigrate.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to init driver: %s", err)
		}

		source, err := iofs.New(migrations, "sqlite/migration")
		if err != nil {
			return nil, fmt.Errorf("failed to embed migrations: %s", err)
		}

		m, err := migrate.NewWithInstance("iofs", source, "fulminedb", driver)
		if err != nil {
			return nil, fmt.Errorf("failed to create migration instance: %s", err)
		}

		_, dirty, verr := m.Version()
		if verr != nil && !errors.Is(verr, migrate.ErrNilVersion) {
			return nil, fmt.Errorf("failed to read migration version: %w", verr)
		}
		if dirty {
			return nil, fmt.Errorf("database is in a dirty migration state; manual intervention required")
		}

		// Apply all pending SQL schema migrations first.
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return nil, fmt.Errorf("failed to run schema migrations: %w", err)
		}

		// Apply all pending Go-side data migrations through the registry.
		if err := ApplyGoMigrations(context.Background(), db, goMigrations); err != nil {
			return nil, fmt.Errorf("failed to run data migrations: %w", err)
		}

		settingsRepo, err = sqlitedb.NewSettingsRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open settings db: %s", err)
		}
		vhtlcRepo, err = sqlitedb.NewVHTLCRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open vhtlc db: %s", err)
		}
		vtxoRolloverRepo, err = sqlitedb.NewVtxoRolloverRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open vtxo rollover db: %s", err)
		}
		swapRepo, err = sqlitedb.NewSwapRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open swap db: %s", err)
		}

		subscribedScriptRepo, err = sqlitedb.NewSubscribedScriptRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open subscribed script db: %s", err)
		}

	default:
		return nil, fmt.Errorf("unsopported db type %s, please select one of %s", config.DbType, allowedTypes)
	}

	return &service{
		settingsRepo:         settingsRepo,
		vhtlcRepo:            vhtlcRepo,
		vtxoRolloverRepo:     vtxoRolloverRepo,
		swapRepo:             swapRepo,
		subscribedScriptRepo: subscribedScriptRepo,
	}, nil
}

func (s *service) Settings() domain.SettingsRepository {
	return s.settingsRepo
}

func (s *service) VHTLC() domain.VHTLCRepository {
	return s.vhtlcRepo
}

func (s *service) VtxoRollover() domain.VtxoRolloverRepository {
	return s.vtxoRolloverRepo
}

func (s *service) Swap() domain.SwapRepository {
	return s.swapRepo
}

func (s *service) SubscribedScript() domain.SubscribedScriptRepository {
	return s.subscribedScriptRepo
}

func (s *service) Close() {
	s.settingsRepo.Close()
	s.vhtlcRepo.Close()
	s.vtxoRolloverRepo.Close()
	s.swapRepo.Close()
	s.subscribedScriptRepo.Close()
}
