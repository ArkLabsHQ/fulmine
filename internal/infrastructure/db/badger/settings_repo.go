package badgerdb

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
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
	LnUrl:       "lndconnect://localhost:10009?macaroon=AgEDbG5kAvgBAwoQutxVspua-pBNR7qKHHmx-BIBMBoWCgdhZGRyZXNzEgRyZWFkEgV3cml0ZRoTCgRpbmZvEgRyZWFkEgV3cml0ZRoXCghpbnZvaWNlcxIEcmVhZBIFd3JpdGUaIQoIbWFjYXJvb24SCGdlbmVyYXRlEgRyZWFkEgV3cml0ZRoWCgdtZXNzYWdlEgRyZWFkEgV3cml0ZRoXCghvZmZjaGFpbhIEcmVhZBIFd3JpdGUaFgoHb25jaGFpbhIEcmVhZBIFd3JpdGUaFAoFcGVlcnMSBHJlYWQSBXdyaXRlGhgKBnNpZ25lchIIZ2VuZXJhdGUSBHJlYWQAAAYggecHfFbYt-pRIIMMp35bu31XBGMCFi2TDh_N3oWoU2g&cert=MIICKTCCAc-gAwIBAgIQN0pV79yC63Ogp0jHwNHIpDAKBggqhkjOPQQDAjA4MR8wHQYDVQQKExZsbmQgYXV0b2dlbmVyYXRlZCBjZXJ0MRUwEwYDVQQDEwwxOTE3NTQ5OWY1N2EwHhcNMjQxMTEyMTAzNjE3WhcNMjYwMTA3MTAzNjE3WjA4MR8wHQYDVQQKExZsbmQgYXV0b2dlbmVyYXRlZCBjZXJ0MRUwEwYDVQQDEwwxOTE3NTQ5OWY1N2EwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQiSdA-iu6DDdUvtu3dCkBz-8H7ZQ_4dOpxbC7THOWxT-r1ulrpMPodpJlyktN3q5rjC2-y89Uozm2DzjIkpraxo4G6MIG3MA4GA1UdDwEB_wQEAwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH_MB0GA1UdDgQWBBTd57yq86dTP4HW3YwyOM4IwmhPDDBgBgNVHREEWTBXggwxOTE3NTQ5OWY1N2GCCWxvY2FsaG9zdIIDbG5kggR1bml4ggp1bml4cGFja2V0ggdidWZjb25uhwR_AAABhxAAAAAAAAAAAAAAAAAAAAABhwTAqGEEMAoGCCqGSM49BAMCA0gAMEUCIQDgzKlajEHTIglruj9YmYnkeP52zMlnDGpHBloNxu70MgIgULA7ni9hWHuMi_tkaHog9olaTiUvW9yJWlRBKpjuSGI",
	Unit:        "sat",
}

type service struct {
	store *badgerhold.Store
}

func NewSettingsRepo(baseDir string, logger badger.Logger) (domain.SettingsRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, settingsDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open round events store: %s", err)
	}
	return &service{store}, nil
}

func (s *service) AddDefaultSettings(ctx context.Context) error {
	return s.addSettings(ctx, defaultSettings)
}

func (s *service) AddSettings(ctx context.Context, settings domain.Settings) error {
	return s.addSettings(ctx, settings)
}

func (s *service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	return s.getSettings(ctx)
}

func (s *service) CleanSettings(ctx context.Context) error {
	return s.deleteSettings(ctx)
}

func (s *service) UpdateSettings(
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

func (s *service) addSettings(
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

func (s *service) updateSettings(
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

func (s *service) getSettings(ctx context.Context) (*domain.Settings, error) {
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

func (s *service) deleteSettings(ctx context.Context) (err error) {
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxDelete(tx, settingsKey, domain.Settings{})
	} else {
		err = s.store.Delete(settingsKey, domain.Settings{})
	}
	return
}
