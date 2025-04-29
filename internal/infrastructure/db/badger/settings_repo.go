package badgerdb

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

const (
	settingsKey = "settings"
	settingsDir = "settings"
)

var defaultSettings = domain.Settings{
	ApiRoot:     "https://fulmine.io/api/D9D90N192031",
	ServerUrl:   "http://localhost:7000",
	Currency:    "usd",
	EventServer: "http://arklabs.to/node/jupiter29",
	FullNode:    "http://arklabs.to/node/213908123",
	Unit:        "sat",
}

type settingsRepository struct {
	store *badgerhold.Store
}

func NewSettingsRepository(baseDir string, logger badger.Logger) (domain.SettingsRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, settingsDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open round events store: %s", err)
	}
	return &settingsRepository{store}, nil
}

func (s *settingsRepository) AddDefaultSettings(ctx context.Context) error {
	return s.addSettings(ctx, defaultSettings)
}

func (s *settingsRepository) AddSettings(ctx context.Context, settings domain.Settings) error {
	return s.addSettings(ctx, settings)
}

func (s *settingsRepository) GetSettings(ctx context.Context) (*domain.Settings, error) {
	return s.getSettings(ctx)
}

func (s *settingsRepository) CleanSettings(ctx context.Context) error {
	return s.deleteSettings(ctx)
}

func (s *settingsRepository) UpdateSettings(
	ctx context.Context, newSettings domain.Settings,
) error {
	settings, err := s.getSettings(ctx)
	if err != nil {
		return err
	}
	if len(newSettings.ApiRoot) > 0 {
		settings.ApiRoot = newSettings.ApiRoot
	}
	if len(newSettings.ServerUrl) > 0 {
		settings.ServerUrl = newSettings.ServerUrl
	}
	if len(newSettings.Currency) > 0 {
		settings.Currency = newSettings.Currency
	}
	if len(newSettings.EventServer) > 0 {
		settings.EventServer = newSettings.EventServer
	}
	if len(newSettings.FullNode) > 0 {
		settings.FullNode = newSettings.FullNode
	}
	if len(newSettings.LnUrl) > 0 {
		settings.LnUrl = newSettings.LnUrl
	}
	if len(newSettings.Unit) > 0 {
		settings.Unit = newSettings.Unit
	}
	return s.updateSettings(ctx, *settings)
}

func (s *settingsRepository) Close() {
	// nolint:all
	s.store.Close()
}

func (s *settingsRepository) addSettings(
	ctx context.Context, settings domain.Settings,
) (err error) {
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxInsert(tx, settingsKey, settings)
	} else {
		err = s.store.Insert(settingsKey, settings)
	}
	return
}

func (s *settingsRepository) updateSettings(
	ctx context.Context, settings domain.Settings,
) (err error) {
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxUpdate(tx, settingsKey, settings)
	} else {
		err = s.store.Update(settingsKey, settings)
	}
	return
}

func (s *settingsRepository) getSettings(ctx context.Context) (*domain.Settings, error) {
	var settings domain.Settings
	var err error
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxGet(tx, settingsKey, &settings)
	} else {
		err = s.store.Get(settingsKey, &settings)
	}
	if err != nil && err == badgerhold.ErrNotFound {
		return nil, fmt.Errorf("settings not found")
	}

	return &settings, nil
}

func (s *settingsRepository) deleteSettings(ctx context.Context) (err error) {
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxDelete(tx, settingsKey, domain.Settings{})
	} else {
		err = s.store.Delete(settingsKey, domain.Settings{})
	}
	return
}
