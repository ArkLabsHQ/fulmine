package utils_test

import (
	"testing"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	"github.com/tyler-smith/go-bip39"
)

// A fixed BIP39 test vector, so the derived key is reproducible.
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestPrivateKeyFromMnemonic(t *testing.T) {
	const (
		hardened = hdkeychain.HardenedKeyStart
		purpose  = hardened + 86
		account  = hardened + 0
	)

	tests := []struct {
		name     string
		network  string
		coinType uint32
	}{
		{name: "mainnet", network: "bitcoin", coinType: hardened + 0},
		{name: "mainnet alias", network: "mainnet", coinType: hardened + 0},
		{name: "regtest", network: "regtest", coinType: hardened + 1},
		{name: "signet", network: "signet", coinType: hardened + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.PrivateKeyFromMnemonic(testMnemonic, tt.network)
			require.NoError(t, err)
			require.NotNil(t, got)

			// The full BIP86 leaf: m/86'/coin'/0'/0/0. The two trailing
			// non-hardened zeros are what go-sdk's key service appends beneath
			// the account root for key index 0.
			leaf := deriveWithHDKeychain(t, testMnemonic, []uint32{
				purpose, tt.coinType, account, 0, 0,
			})
			wantPriv, err := leaf.ECPrivKey()
			require.NoError(t, err)

			require.Equal(t, wantPriv.Serialize(), got.Serialize(),
				"expected the key at m/86'/coin'/0'/0/0")
		})
	}
}

// deriveWithHDKeychain walks a derivation path using btcd's hdkeychain, which is
// a different implementation from the go-bip32 one under test. Agreement between
// the two is meaningful; comparing the implementation against itself would not
// be.
func deriveWithHDKeychain(t *testing.T, mnemonic string, path []uint32) *hdkeychain.ExtendedKey {
	t.Helper()

	seed := bip39.NewSeed(mnemonic, "")
	key, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	require.NoError(t, err)

	for _, step := range path {
		key, err = key.Derive(step)
		require.NoError(t, err)
	}
	return key
}
