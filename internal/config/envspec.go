package config

import "fmt"

type EnvVar struct {
	Name        string // short name under the FULMINE_ prefix (e.g., "DATADIR")
	FullName    string // e.g., "FULMINE_DATADIR"
	Type        string // human-readable type
	Default     string // default value as a string ("" if none)
	Description string // one-liner for docs
	Notes       string // optional: constraints, examples, etc.
}

func EnvSpecs() []EnvVar {
	const P = "FULMINE_"

	return []EnvVar{
		{
			Name:        Datadir,
			FullName:    P + Datadir,
			Type:        "string (path)",
			Default:     DefaultDatadir,
			Description: "Data directory for Fulmine state",
		},
		{
			Name:        DbType,
			FullName:    P + DbType,
			Type:        "string",
			Default:     DefaultDbType, // "sqlite" by default
			Description: "Database backend: sqlite | badger",
		},
		{
			Name:        GRPCPort,
			FullName:    P + GRPCPort,
			Type:        "uint32 (port)",
			Default:     fmt.Sprintf("%d", DefaultGRPCPort),
			Description: "gRPC server port",
		},
		{
			Name:        HTTPPort,
			FullName:    P + HTTPPort,
			Type:        "uint32 (port)",
			Default:     fmt.Sprintf("%d", DefaultHTTPPort),
			Description: "HTTP server port",
		},
		{
			Name:        WithTLS,
			FullName:    P + WithTLS,
			Type:        "bool",
			Default:     fmt.Sprintf("%v", DefaultWithTLS),
			Description: "Enable TLS on server",
		},
		{
			Name:        LogLevel,
			FullName:    P + LogLevel,
			Type:        "uint32 (0–5)",
			Default:     fmt.Sprintf("%d", DefaultLogLevel),
			Description: "Log verbosity (higher = more verbose)",
		},
		{
			Name:        ArkServer,
			FullName:    P + ArkServer,
			Type:        "string (host:port)",
			Default:     DefaultArkServer, // "" means unset
			Description: "Ark server address (e.g., arkd:7070)",
		},
		{
			Name:        EsploraURL,
			FullName:    P + EsploraURL,
			Type:        "string (URL)",
			Default:     "",
			Description: "Esplora base URL (e.g., http://chopsticks:3000)",
		},
		{
			Name:        BoltzURL,
			FullName:    P + BoltzURL,
			Type:        "string (URL)",
			Default:     "",
			Description: "Boltz HTTP endpoint (e.g., http://boltz:9001)",
		},
		{
			Name:        BoltzWSURL,
			FullName:    P + BoltzWSURL,
			Type:        "string (WS URL)",
			Default:     "",
			Description: "Boltz WebSocket endpoint (e.g., ws://boltz:9004)",
		},
		{
			Name:        DisableTelemetry,
			FullName:    P + DisableTelemetry,
			Type:        "bool",
			Default:     fmt.Sprintf("%v", DefaultDisableTelemetry),
			Description: "Disable telemetry collection",
		},
		{
			Name:        NoMacaroons,
			FullName:    P + NoMacaroons,
			Type:        "bool",
			Default:     fmt.Sprintf("%v", DefaultNoMacaroons),
			Description: "Disable macaroon authentication (if true)",
			Notes:       "If false, macaroons are stored under DATADIR and enforced on RPCs.",
		},
		// --- Lightning connection options (mutually exclusive where noted) ---
		{
			Name:        LndUrl,
			FullName:    P + LndUrl,
			Type:        "string (URL or lndconnect://)",
			Default:     DefaultLndUrl,
			Description: "LND connection: lndconnect://… or https://host:port",
			Notes:       "Cannot be set with CLN_URL. If not lndconnect://, LND_DATADIR must be set.",
		},
		{
			Name:        LndDatadir,
			FullName:    P + LndDatadir,
			Type:        "string (path)",
			Default:     DefaultLndDatadir,
			Description: "Path to LND data directory (required when LND_URL is https://…)",
		},
		{
			Name:        ClnUrl,
			FullName:    P + ClnUrl,
			Type:        "string (URL or clnconnect://)",
			Default:     DefaultClnUrl,
			Description: "CLN connection: clnconnect://… or https://host:port",
			Notes:       "Cannot be set with LND_URL. If not clnconnect://, CLN_DATADIR must be set.",
		},
		{
			Name:        ClnDatadir,
			FullName:    P + ClnDatadir,
			Type:        "string (path)",
			Default:     DefaultClnDatadir,
			Description: "Path to CLN data directory (required when CLN_URL is https://…)",
		},
		// --- Unlocker options ---
		{
			Name:        UnlockerType,
			FullName:    P + UnlockerType,
			Type:        "string",
			Default:     "",
			Description: "Unlocker backend: file | env",
			Notes:       "file → use UNLOCKER_FILE_PATH; env → use UNLOCKER_PASSWORD",
		},
		{
			Name:        UnlockerFilePath,
			FullName:    P + UnlockerFilePath,
			Type:        "string (path)",
			Default:     "",
			Description: "Path to file containing password (when UNLOCKER_TYPE=file)",
		},
		{
			Name:        UnlockerPassword,
			FullName:    P + UnlockerPassword,
			Type:        "string",
			Default:     "",
			Description: "Password from environment (when UNLOCKER_TYPE=env)",
		},
	}
}

//go:generate go run ../../tools/gen-env-doc/main.go
