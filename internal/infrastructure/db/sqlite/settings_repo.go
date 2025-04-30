package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
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
	db      *sql.DB
	querier *queries.Queries
}

func NewSettingsRepository(db *sql.DB) (domain.SettingsRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open settings repository: db is nil")
	}
	return &settingsRepository{db: db, querier: queries.New(db)}, nil
}

func (s *settingsRepository) AddDefaultSettings(ctx context.Context) error {
	return s.AddSettings(ctx, defaultSettings)
}

func (s *settingsRepository) AddSettings(ctx context.Context, settings domain.Settings) error {
	_, err := s.GetSettings(ctx)
	if err == nil {
		return fmt.Errorf("settings already exist")
	}
	var esplora sql.NullString
	if settings.EsploraUrl != "" {
		esplora = sql.NullString{String: settings.EsploraUrl, Valid: true}
	}
	var lnurl sql.NullString
	if settings.LnUrl != "" {
		lnurl = sql.NullString{String: settings.LnUrl, Valid: true}
	}
	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     settings.ApiRoot,
		ServerUrl:   settings.ServerUrl,
		EsploraUrl:  esplora,
		Currency:    settings.Currency,
		EventServer: settings.EventServer,
		FullNode:    settings.FullNode,
		LnUrl:       lnurl,
		Unit:        settings.Unit,
	})
}

func (s *settingsRepository) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	existing, err := s.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("settings not found")
	}

	merged := *existing
	if settings.ApiRoot != "" {
		merged.ApiRoot = settings.ApiRoot
	}
	if settings.ServerUrl != "" {
		merged.ServerUrl = settings.ServerUrl
	}
	if settings.EsploraUrl != "" {
		merged.EsploraUrl = settings.EsploraUrl
	}
	if settings.Currency != "" {
		merged.Currency = settings.Currency
	}
	if settings.EventServer != "" {
		merged.EventServer = settings.EventServer
	}
	if settings.FullNode != "" {
		merged.FullNode = settings.FullNode
	}
	if settings.LnUrl != "" {
		merged.LnUrl = settings.LnUrl
	}
	if settings.Unit != "" {
		merged.Unit = settings.Unit
	}

	var esplora sql.NullString
	if merged.EsploraUrl != "" {
		esplora = sql.NullString{String: merged.EsploraUrl, Valid: true}
	}
	var lnurl sql.NullString
	if merged.LnUrl != "" {
		lnurl = sql.NullString{String: merged.LnUrl, Valid: true}
	}
	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     merged.ApiRoot,
		ServerUrl:   merged.ServerUrl,
		EsploraUrl:  esplora,
		Currency:    merged.Currency,
		EventServer: merged.EventServer,
		FullNode:    merged.FullNode,
		LnUrl:       lnurl,
		Unit:        merged.Unit,
	})
}

func (s *settingsRepository) GetSettings(ctx context.Context) (*domain.Settings, error) {
	row, err := s.querier.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	settings := &domain.Settings{
		ApiRoot:     row.ApiRoot,
		ServerUrl:   row.ServerUrl,
		Currency:    row.Currency,
		EventServer: row.EventServer,
		FullNode:    row.FullNode,
		Unit:        row.Unit,
	}
	if row.EsploraUrl.Valid {
		settings.EsploraUrl = row.EsploraUrl.String
	}
	if row.LnUrl.Valid {
		settings.LnUrl = row.LnUrl.String
	}
	return settings, nil
}

func (s *settingsRepository) CleanSettings(ctx context.Context) error {
	_, err := s.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("settings not found")
	}
	return s.querier.DeleteSettings(ctx)
}

func (s *settingsRepository) Close() {
	if s.db != nil {
		s.db.Close()
	}
}
