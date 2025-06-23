package domain

import (
	"context"
)

type LnConnectionOpts struct {
	LndMacaroonPath   string `json:"lnd_macaroon_path"`
	TlsCertPath       string `json:"tls_cert_path"`
	ClnRootCertPath   string `json:"cln_root_cert_path"`
	ClnPrivateKeyPath string `json:"cln_private_key_path"`
	ClnCertChainPath  string `json:"cln_cert_chain_path"`
	Host              string `json:"host"`
}

type Settings struct {
	ApiRoot        string
	ServerUrl      string
	EsploraUrl     string
	Currency       string
	EventServer    string
	FullNode       string
	LnUrl          string
	Unit           string
	ConnectionOpts *LnConnectionOpts
}

type SettingsRepository interface {
	AddDefaultSettings(ctx context.Context) error
	AddSettings(ctx context.Context, settings Settings) error
	GetSettings(ctx context.Context) (*Settings, error)
	CleanSettings(ctx context.Context) error
	UpdateSettings(ctx context.Context, settings Settings) error
	Close()
}
