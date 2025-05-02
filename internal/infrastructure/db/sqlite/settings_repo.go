package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
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
		return nil, fmt.Errorf("missing db")
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
	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     settings.ApiRoot,
		ServerUrl:   settings.ServerUrl,
		EsploraUrl:  sql.NullString{String: settings.EsploraUrl, Valid: true},
		Currency:    settings.Currency,
		EventServer: settings.EventServer,
		FullNode:    settings.FullNode,
		LnUrl:       sql.NullString{String: settings.LnUrl, Valid: true},
		Unit:        settings.Unit,
	})
}

func (s *settingsRepository) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	existing, err := s.GetSettings(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("settings not found")
		}
		return err
	}

	if settings.ApiRoot != "" {
		existing.ApiRoot = settings.ApiRoot
	}
	if settings.ServerUrl != "" {
		existing.ServerUrl = settings.ServerUrl
	}
	if settings.EsploraUrl != "" {
		existing.EsploraUrl = settings.EsploraUrl
	}
	if settings.Currency != "" {
		existing.Currency = settings.Currency
	}
	if settings.EventServer != "" {
		existing.EventServer = settings.EventServer
	}
	if settings.FullNode != "" {
		existing.FullNode = settings.FullNode
	}
	if settings.LnUrl != "" {
		existing.LnUrl = settings.LnUrl
	}
	if settings.Unit != "" {
		existing.Unit = settings.Unit
	}
	if settings.EsploraUrl != "" {
		existing.EsploraUrl = settings.EsploraUrl
	}
	if settings.LnUrl != "" {
		existing.LnUrl = settings.LnUrl
	}
	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     existing.ApiRoot,
		ServerUrl:   existing.ServerUrl,
		EsploraUrl:  sql.NullString{String: existing.EsploraUrl, Valid: true},
		Currency:    existing.Currency,
		EventServer: existing.EventServer,
		FullNode:    existing.FullNode,
		LnUrl:       sql.NullString{String: existing.LnUrl, Valid: true},
		Unit:        existing.Unit,
	})
}

func (s *settingsRepository) GetSettings(ctx context.Context) (*domain.Settings, error) {
	row, err := s.querier.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.Settings{
		ApiRoot:     row.ApiRoot,
		ServerUrl:   row.ServerUrl,
		Currency:    row.Currency,
		EventServer: row.EventServer,
		FullNode:    row.FullNode,
		Unit:        row.Unit,
		EsploraUrl:  row.EsploraUrl.String,
		LnUrl:       row.LnUrl.String,
	}, nil
}

func (s *settingsRepository) CleanSettings(ctx context.Context) error {
	_, err := s.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("settings not found")
	}
	if err := s.querier.DeleteSettings(ctx); err != nil {
		return err
	}
	// nolint:all
	s.db.ExecContext(ctx, "VACUUM")
	return nil
}

func (s *settingsRepository) Close() {
	if s.db != nil {
		s.db.Close()
	}
}
