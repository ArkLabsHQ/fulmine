//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"reflect"

	"github.com/ArkLabsHQ/fulmine/internal/config"
)

func main() {
	t := reflect.TypeOf(config.Config{})

	md := "# Environment Variables\n\n" +
		"Generated from `config Structure`. **Do not edit manually.**\n\n" +
		"| Variable | Default | Type | Description |\n" +
		"|----------|--------|------|-------------|\n"

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		mapTag := f.Tag.Get("mapstructure")
		if mapTag == "" {
			panic(fmt.Sprintf("field %s missing mapstructure tag", f.Name))
		}
		if mapTag == "-" {
			// Derived/non-env field (e.g. LnConnectionOpts); the env vars it is
			// built from are documented in the extras list below.
			continue
		}

		key := "FULMINE_" + mapTag
		def := f.Tag.Get("envDefault")
		info := f.Tag.Get("envInfo")
		if info == "" {
			panic(fmt.Sprintf("field %s missing envInfo tag", f.Name))
		}

		envType := f.Type.String()

		md += fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n", key, def, envType, info)

	}

	// Env vars read directly via viper rather than decoded into a Config field:
	// the Lightning settings are derived into LnConnectionOpts, and NoMacaroons
	// is consumed at wiring time.
	extras := [][4]string{
		{"FULMINE_NO_MACAROONS", "false", "bool", "Disable macaroons"},
		{"FULMINE_LND_URL", "", "string", "LND connection URL (lndconnect:// or http://host:port)"},
		{"FULMINE_CLN_URL", "", "string", "CLN connection URL (clnconnect:// or http://host:port)"},
		{"FULMINE_CLN_DATADIR", "", "string", "CLN data directory (required if not using clnconnect://)"},
		{"FULMINE_LND_DATADIR", "", "string", "LND data directory (required if not using lndconnect://)"},
	}
	for _, e := range extras {
		md += fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n", e[0], e[1], e[2], e[3])
	}

	if err := os.MkdirAll("../../docs", 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("../../docs/environment.md", []byte(md), 0o644); err != nil {
		panic(err)
	}
}
