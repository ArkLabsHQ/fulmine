package domain

import (
	"context"
)

type ConnectionType int

const (
	CLN_CONNECTION ConnectionType = iota
	LND_CONNECTION
)

type LnConnectionOpts struct {
	LnDatadir      string
	LnUrl          string
	ConnectionType ConnectionType
}

type Settings struct {
	ApiRoot          string
	ServerUrl        string
	EsploraUrl       string
	Currency         string
	EventServer      string
	FullNode         string
	Unit             string
	LnConnectionOpts *LnConnectionOpts
}

type SettingsRepository interface {
	AddDefaultSettings(ctx context.Context) error
	AddSettings(ctx context.Context, settings Settings) error
	GetSettings(ctx context.Context) (*Settings, error)
	CleanSettings(ctx context.Context) error
	UpdateSettings(ctx context.Context, settings Settings) error
	Close()
}
