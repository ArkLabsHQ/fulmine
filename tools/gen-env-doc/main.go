//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"

	cfg "github.com/ArkLabsHQ/fulmine/internal/config" // <-- update to your actual package path
)

func main() {
	specs := cfg.EnvSpecs()

	md := "# Environment Variables\n\n" +
		"Generated from `config.EnvSpecs()`. **Do not edit manually.**\n\n" +
		"| Variable | Default | Type | Description |\n" +
		"|----------|--------|------|-------------|\n"

	for _, s := range specs {
		def := s.Default
		if def == "" {
			def = "—"
		}
		desc := s.Description
		if s.Notes != "" {
			desc += "<br/><em>" + s.Notes + "</em>"
		}
		md += fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n", s.FullName, def, s.Type, desc)
	}

	if err := os.MkdirAll("../../docs", 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("../../docs/environment.md", []byte(md), 0o644); err != nil {
		panic(err)
	}
}
