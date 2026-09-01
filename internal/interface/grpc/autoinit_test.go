package grpc_interface

import (
	"strings"
	"testing"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/stretchr/testify/require"
)

const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestResolveSetupMnemonic(t *testing.T) {
	t.Run("configured mnemonic is echoed", func(t *testing.T) {
		mnemonic, generated, err := resolveSetupMnemonic(testMnemonic)
		require.NoError(t, err)
		require.False(t, generated)
		require.Equal(t, testMnemonic, mnemonic)
	})

	t.Run("empty generates a valid mnemonic", func(t *testing.T) {
		mnemonic, generated, err := resolveSetupMnemonic("")
		require.NoError(t, err)
		require.True(t, generated)
		require.NoError(t, utils.IsValidMnemonic(mnemonic))
	})
}

func TestNormalizeMnemonic(t *testing.T) {
	require.Equal(t,
		normalizeMnemonic("Word  other\tthing\n"),
		normalizeMnemonic("word other thing"),
	)
	require.NotEqual(t,
		normalizeMnemonic("word other thing"),
		normalizeMnemonic("word other things"),
	)
}

func TestFormatMnemonicBanner(t *testing.T) {
	banner := formatMnemonicBanner(testMnemonic)
	require.Contains(t, banner, testMnemonic)
	require.Contains(t, banner, "BACK UP YOUR MNEMONIC")
	require.Contains(t, banner, "ONCE")
	require.GreaterOrEqual(t, strings.Count(banner, "====="), 2)
}
