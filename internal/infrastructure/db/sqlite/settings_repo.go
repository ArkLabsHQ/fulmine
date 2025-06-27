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

	lnType := sql.NullInt64{Valid: false}
	lnDatadir := sql.NullString{Valid: false}
	lnUrl := sql.NullString{Valid: false}
	if settings.LnConnectionOpts != nil {
		lnType = sql.NullInt64{Int64: int64(settings.LnConnectionOpts.ConnectionType), Valid: true}
		lnDatadir = sql.NullString{String: settings.LnConnectionOpts.LnDatadir, Valid: true}
		lnUrl = sql.NullString{String: settings.LnConnectionOpts.LnUrl, Valid: true}
	}

	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     settings.ApiRoot,
		ServerUrl:   settings.ServerUrl,
		EsploraUrl:  sql.NullString{String: settings.EsploraUrl, Valid: true},
		Currency:    settings.Currency,
		EventServer: settings.EventServer,
		FullNode:    settings.FullNode,
		Unit:        settings.Unit,
		LnUrl:       lnUrl,
		LnDatadir:   lnDatadir,
		LnType:      lnType,
	})
}

func (s *settingsRepository) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	existing, err := s.querier.GetSettings(ctx)
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

	if settings.Currency != "" {
		existing.Currency = settings.Currency
	}
	if settings.EventServer != "" {
		existing.EventServer = settings.EventServer
	}
	if settings.FullNode != "" {
		existing.FullNode = settings.FullNode
	}

	if settings.Unit != "" {
		existing.Unit = settings.Unit
	}
	if settings.EsploraUrl != "" {
		existing.EsploraUrl = sql.NullString{String: settings.EsploraUrl, Valid: true}
	}

	if settings.LnConnectionOpts != nil {
		existing.LnType = sql.NullInt64{Int64: int64(settings.LnConnectionOpts.ConnectionType), Valid: true}
		existing.LnDatadir = sql.NullString{String: settings.LnConnectionOpts.LnDatadir, Valid: true}
		existing.LnUrl = sql.NullString{String: settings.LnConnectionOpts.LnUrl, Valid: true}
	}

	return s.querier.UpsertSettings(ctx, queries.UpsertSettingsParams{
		ApiRoot:     existing.ApiRoot,
		ServerUrl:   existing.ServerUrl,
		EsploraUrl:  existing.EsploraUrl,
		Currency:    existing.Currency,
		EventServer: existing.EventServer,
		FullNode:    existing.FullNode,
		Unit:        existing.Unit,
		LnUrl:       existing.LnUrl,
		LnDatadir:   existing.LnDatadir,
		LnType:      existing.LnType,
	})
}

func (s *settingsRepository) GetSettings(ctx context.Context) (*domain.Settings, error) {
	row, err := s.querier.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	var lnConnectionOpts *domain.LnConnectionOpts

	if row.LnType.Valid {
		lnConnectionOpts = &domain.LnConnectionOpts{
			ConnectionType: domain.ConnectionType(row.LnType.Int64),
			LnDatadir:      row.LnDatadir.String,
			LnUrl:          row.LnUrl.String,
		}
	}

	return &domain.Settings{
		ApiRoot:          row.ApiRoot,
		ServerUrl:        row.ServerUrl,
		Currency:         row.Currency,
		EventServer:      row.EventServer,
		FullNode:         row.FullNode,
		Unit:             row.Unit,
		EsploraUrl:       row.EsploraUrl.String,
		LnConnectionOpts: lnConnectionOpts,
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
