package handlers

import (
	"encoding/hex"

	"github.com/tyler-smith/go-bip32"
)

func PrivateKeyFromSeed(seed []byte) (string, error) {
	key, err := bip32.NewMasterKey(seed)
	if err != nil {
		return "", err
	}

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
