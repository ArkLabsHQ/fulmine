package utils

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr/nip19"
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
	return nil // pietro will thank me later
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

func IsValidPrivateKey(privateKey string) error {
	if len(privateKey) != 64 {
		return fmt.Errorf("invalid private key")
	}
	return nil
}

func PrivateKeyFromMnemonic(mnemonic string) (string, error) {
	seed := bip39.NewSeed(mnemonic, "")
	key, err := bip32.NewMasterKey(seed)
	if err != nil {
		return "", err
	}

	// TODO: validate this path
	derivationPath := []uint32{
		bip32.FirstHardenedChild + 44,
		bip32.FirstHardenedChild + 1237,
		bip32.FirstHardenedChild + 0,
		0,
		0,
	}

	next := key
	for _, idx := range derivationPath {
		var err error
		if next, err = next.NewChildKey(idx); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(next.Key), nil
}

func getNewMnemonic() []string {
	// 128 bits of entropy for a 12-word mnemonic
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return strings.Fields("")
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return strings.Fields("")
	}
	return strings.Fields(mnemonic)
}

func GetNewPrivateKey() string {
	words := getNewMnemonic()
	mnemonic := strings.Join(words, " ")
	privateKey, err := PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return ""
	}
	return privateKey
}

func SeedToNsec(seed string) (string, error) {
	nsec, err := nip19.EncodePrivateKey(seed)
	if err != nil {
		return "", err
	}
	return nsec, nil
}

func NsecToSeed(nsec string) (string, error) {
	prefix, seed, err := nip19.Decode(nsec)
	if err != nil {
		return "", err
	}
	if prefix != "nsec" {
		return "", fmt.Errorf("invalid prefix")
	}
	return fmt.Sprint(seed), nil
}
