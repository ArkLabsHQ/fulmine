package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const validMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestResolveAutoInitMnemonic(t *testing.T) {
	writeFile := func(t *testing.T, content string) string {
		path := filepath.Join(t.TempDir(), "mnemonic")
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		return path
	}

	t.Run("neither set returns empty", func(t *testing.T) {
		mnemonic, err := resolveAutoInitMnemonic("", "")
		require.NoError(t, err)
		require.Empty(t, mnemonic)
	})

	t.Run("both set are mutually exclusive", func(t *testing.T) {
		_, err := resolveAutoInitMnemonic(validMnemonic, writeFile(t, validMnemonic))
		require.ErrorContains(t, err, "mutually exclusive")
	})

	t.Run("valid mnemonic from env", func(t *testing.T) {
		mnemonic, err := resolveAutoInitMnemonic(validMnemonic, "")
		require.NoError(t, err)
		require.Equal(t, validMnemonic, mnemonic)
	})

	t.Run("mnemonic from file is normalized", func(t *testing.T) {
		path := writeFile(t, "  Abandon abandon abandon\tabandon abandon abandon abandon abandon abandon abandon abandon about\r\n")
		mnemonic, err := resolveAutoInitMnemonic("", path)
		require.NoError(t, err)
		require.Equal(t, validMnemonic, mnemonic)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := resolveAutoInitMnemonic("", filepath.Join(t.TempDir(), "does-not-exist"))
		require.ErrorContains(t, err, "failed to read mnemonic file")
	})

	t.Run("invalid mnemonic", func(t *testing.T) {
		_, err := resolveAutoInitMnemonic("abandon abandon abandon", "")
		require.ErrorContains(t, err, "invalid auto-init mnemonic")
	})

	t.Run("bad checksum", func(t *testing.T) {
		_, err := resolveAutoInitMnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon", "")
		require.ErrorContains(t, err, "invalid auto-init mnemonic")
	})
}

func TestLoadConfigAutoInit(t *testing.T) {
	load := func(t *testing.T, env map[string]string) (*Config, error) {
		viper.Reset()
		t.Setenv("FULMINE_DATADIR", t.TempDir())
		t.Setenv("FULMINE_NO_MACAROONS", "true")
		for k, v := range env {
			t.Setenv(k, v)
		}
		return LoadConfig()
	}

	t.Run("auto-init requires an unlocker", func(t *testing.T) {
		_, err := load(t, map[string]string{
			"FULMINE_AUTO_INIT":  "true",
			"FULMINE_ARK_SERVER": "http://localhost:7070",
		})
		require.ErrorContains(t, err, "requires an unlocker")
	})

	t.Run("auto-init requires ark server", func(t *testing.T) {
		_, err := load(t, map[string]string{
			"FULMINE_AUTO_INIT":         "true",
			"FULMINE_UNLOCKER_TYPE":     "env",
			"FULMINE_UNLOCKER_PASSWORD": "password",
		})
		require.ErrorContains(t, err, "requires FULMINE_ARK_SERVER")
	})

	t.Run("mnemonic requires auto-init", func(t *testing.T) {
		_, err := load(t, map[string]string{
			"FULMINE_MNEMONIC": validMnemonic,
		})
		require.ErrorContains(t, err, "require FULMINE_AUTO_INIT=true")
	})

	t.Run("valid auto-init without mnemonic", func(t *testing.T) {
		cfg, err := load(t, map[string]string{
			"FULMINE_AUTO_INIT":         "true",
			"FULMINE_ARK_SERVER":        "http://localhost:7070",
			"FULMINE_UNLOCKER_TYPE":     "env",
			"FULMINE_UNLOCKER_PASSWORD": "password",
		})
		require.NoError(t, err)
		require.True(t, cfg.AutoInit)
		require.Empty(t, cfg.Mnemonic)
	})

	t.Run("valid auto-init with mnemonic", func(t *testing.T) {
		cfg, err := load(t, map[string]string{
			"FULMINE_AUTO_INIT":         "true",
			"FULMINE_ARK_SERVER":        "http://localhost:7070",
			"FULMINE_UNLOCKER_TYPE":     "env",
			"FULMINE_UNLOCKER_PASSWORD": "password",
			"FULMINE_MNEMONIC":          validMnemonic + "\n",
		})
		require.NoError(t, err)
		require.True(t, cfg.AutoInit)
		require.Equal(t, validMnemonic, cfg.Mnemonic)
	})

	t.Run("disabled auto-init is unchanged", func(t *testing.T) {
		cfg, err := load(t, nil)
		require.NoError(t, err)
		require.False(t, cfg.AutoInit)
	})
}
