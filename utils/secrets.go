package utils

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
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
	if len(password) < 8 {
		return fmt.Errorf("password too short")
	}
	numberRegex := regexp.MustCompile(`[0-9]`)
	if !numberRegex.MatchString(password) {
		return fmt.Errorf("password must have a number")
	}
	specialCharRegex := regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>]`)
	if !specialCharRegex.MatchString(password) {
		return fmt.Errorf("password must have a special character")
	}
	return nil
}

func IsValidPrivateKey(privateKey string) error {
	logrus.Infof("private key %d %s", len(privateKey), privateKey)
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
