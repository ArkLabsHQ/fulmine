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

		key := "FULMINE_" + mapTag
		def := f.Tag.Get("envDefault")
		info := f.Tag.Get("envInfo")
		if info == "" {
			panic(fmt.Sprintf("field %s missing envInfo tag", f.Name))
		}

		envType := f.Type.String()

		md += fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n", key, def, envType, info)

	}

	if err := os.MkdirAll("../../docs", 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("../../docs/environment.md", []byte(md), 0o644); err != nil {
		panic(err)
	}
}
