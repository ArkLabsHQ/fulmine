package db

import (
	"fmt"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	badgerdb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/badger"
	"github.com/dgraph-io/badger/v4"
)

var (
	allowedTypes = strings.Join([]string{"badger"}, ",")
)

type ServiceConfig struct {
	DbType   string
	DbConfig []any
}

type service struct {
	settingsRepo     domain.SettingsRepository
	vhtlcRepo        domain.VHTLCRepository
	vtxoRolloverRepo domain.VtxoRolloverRepository
}

func NewService(config ServiceConfig) (ports.RepoManager, error) {
	var (
		settingsRepo     domain.SettingsRepository
		vhtlcRepo        domain.VHTLCRepository
		vtxoRolloverRepo domain.VtxoRolloverRepository
		err              error
	)
	switch config.DbType {
	case "badger":
		if len(config.DbConfig) != 2 {
			return nil, fmt.Errorf("badger db config must have 1 element, got %d", len(config.DbConfig))
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
			return nil, fmt.Errorf("failed to open settings db: %s", err)
		}
		vtxoRolloverRepo, err = badgerdb.NewVtxoRolloverRepository(baseDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to open settings db: %s", err)
		}
	default:
		return nil, fmt.Errorf("unsopported db type %s, please select one of %s", config.DbType, allowedTypes)
	}

	return &service{
		settingsRepo:     settingsRepo,
		vhtlcRepo:        vhtlcRepo,
		vtxoRolloverRepo: vtxoRolloverRepo,
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

func (s *service) Close() {
	s.settingsRepo.Close()
	s.vhtlcRepo.Close()
	s.vtxoRolloverRepo.Close()
}
