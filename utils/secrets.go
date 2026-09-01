package utils

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

func IsValidMnemonic(mnemonic string) error {
	words := strings.Fields(mnemonic)
	if len(words) != 12 {
		return fmt.Errorf("must have 12 words")
	}
	if !bip39.IsMnemonicValid(mnemonic) {
		return fmt.Errorf("invalid mnemonic")
	}
	return nil
}

func IsValidPassword(password string) error {
	return nil // TODO: revert
	// if len(password) < 8 {
	// 	return fmt.Errorf("password too short")
	// }
	// numberRegex := regexp.MustCompile(`[0-9]`)
	// if !numberRegex.MatchString(password) {
	// 	return fmt.Errorf("password must have a number")
	// }
	// specialCharRegex := regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>]`)
	// if !specialCharRegex.MatchString(password) {
	// 	return fmt.Errorf("password must have a special character")
	// }
	// return nil
}

func GetNewMnemonic() (string, error) {
	// 128 bits of entropy for a 12-word mnemonic
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}
	return mnemonic, nil
}

// PrivateKeyFromMnemonic returns the private key at path m/86'/coin'/0'/0/0 from the provided
// mnemonic
func PrivateKeyFromMnemonic(mnemonic, network string) (*btcec.PrivateKey, error) {
	seed := bip39.NewSeed(mnemonic, "")
	key, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, err
	}

	next := key
	derivationPath := getBIP86DerivationPath(network)
	for _, idx := range derivationPath {
		var err error
		if next, err = next.NewChildKey(idx); err != nil {
			return nil, err
		}
	}

	privateKey, _ := btcec.PrivKeyFromBytes(next.Key)
	return privateKey, nil
}

func getBIP86DerivationPath(network string) []uint32 {
	coinType := uint32(1)
	if network == "bitcoin" || network == "mainnet" {
		coinType = uint32(0)
	}
	// m/86'/0'/0'/0/0 on mainnet
	// m/86'/1'/0'/0/0 on any other network
	return []uint32{
		hdkeychain.HardenedKeyStart + 86,
		uint32(hdkeychain.HardenedKeyStart) + coinType,
		hdkeychain.HardenedKeyStart,
		0, 0,
	}
}
