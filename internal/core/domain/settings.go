package domain

import "context"

type Settings struct {
	ApiRoot     string
	AspUrl      string
	Currency    string
	EventServer string
	FullNode    string
	LnUrl       string
	Unit        string
}

type SettingsRepository interface {
	AddDefaultSettings(ctx context.Context) error
	AddSettings(ctx context.Context, settings Settings) error
	GetSettings(ctx context.Context) (*Settings, error)
	CleanSettings(ctx context.Context) error
	UpdateSettings(ctx context.Context, settings Settings) error
}
