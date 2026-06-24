package db

import (
	"context"
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

type ServiceConfig struct {
	DbType   string
	DbConfig []any
}

type service struct {
	settingsRepo         domain.SettingsRepository
	vhtlcRepo            domain.VHTLCRepository
	delegateRepo         domain.DelegateRepository
	swapRepo             domain.SwapRepository
	subscribedScriptRepo domain.SubscribedScriptRepository
	chainSwapRepo        domain.ChainSwapRepository
}

func NewService(config ServiceConfig) (ports.RepoManager, error) {
	var (
		settingsRepo         domain.SettingsRepository
		vhtlcRepo            domain.VHTLCRepository
		delegateRepo         domain.DelegateRepository
		swapRepo             domain.SwapRepository
		subscribedScriptRepo domain.SubscribedScriptRepository
		chainSwapRepo        domain.ChainSwapRepository
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
		delegateRepo, err = badgerdb.NewDelegateRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open delegate db: %s", err)
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

		// ---- STEPWISE MIGRATION ----
		vhtlcMigrationBegin := uint(20250622101533)
		version, dirty, verr := m.Version()
		if verr != nil && !errors.Is(verr, migrate.ErrNilVersion) {
			return nil, fmt.Errorf("failed to read migration version: %w", verr)
		}
		if dirty {
			return nil, fmt.Errorf("database is in a dirty migration state; manual intervention required")
		}

		if version < vhtlcMigrationBegin {
			if err := m.Migrate(vhtlcMigrationBegin); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return nil, fmt.Errorf("failed to run migrations: %s", err)
			}

			err = sqlitedb.BackfillVhtlc(context.Background(), db)
			if err != nil {
				return nil, err
			}
		}

		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return nil, fmt.Errorf("failed to run remaining migrations: %s", err)
		}

		settingsRepo, err = sqlitedb.NewSettingsRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open settings db: %s", err)
		}
		vhtlcRepo, err = sqlitedb.NewVHTLCRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open vhtlc db: %s", err)
		}
		delegateRepo, err = sqlitedb.NewDelegateRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open delegate db: %s", err)
		}
		swapRepo, err = sqlitedb.NewSwapRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open swap db: %s", err)
		}

		subscribedScriptRepo, err = sqlitedb.NewSubscribedScriptRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open subscribed script db: %s", err)
		}

		chainSwapRepo, err = sqlitedb.NewChainSwapRepository(db)
		if err != nil {
			return nil, fmt.Errorf("failed to open chain swap db: %s", err)
		}

	default:
		return nil, fmt.Errorf("unsopported db type %s, please select one of %s", config.DbType, allowedTypes)
	}

	return &service{
		settingsRepo:         settingsRepo,
		vhtlcRepo:            vhtlcRepo,
		delegateRepo:         delegateRepo,
		swapRepo:             swapRepo,
		subscribedScriptRepo: subscribedScriptRepo,
		chainSwapRepo:        chainSwapRepo,
	}, nil
}

func (s *service) Settings() domain.SettingsRepository {
	return s.settingsRepo
}

func (s *service) VHTLC() domain.VHTLCRepository {
	return s.vhtlcRepo
}

func (s *service) Delegate() domain.DelegateRepository {
	return s.delegateRepo
}

func (s *service) Swap() domain.SwapRepository {
	return s.swapRepo
}

func (s *service) SubscribedScript() domain.SubscribedScriptRepository {
	return s.subscribedScriptRepo
}

func (s *service) ChainSwaps() domain.ChainSwapRepository {
	return s.chainSwapRepo
}

func (s *service) Close() {
	s.settingsRepo.Close()
	s.vhtlcRepo.Close()
	s.delegateRepo.Close()
	s.swapRepo.Close()
	s.subscribedScriptRepo.Close()
}
