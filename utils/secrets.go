package utils

import (
	"fmt"
	"regexp"
	"strings"

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
