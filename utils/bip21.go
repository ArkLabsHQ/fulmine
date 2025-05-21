package utils

import (
	"regexp"
	"strconv"
	"strings"
)

func IsBip21(text string) bool {
	if !startsWithBitcoinPrefix(text) {
		return false
	}
	invoice := GetInvoice(text)
	onchainAddr := GetBtcAddress(text)
	offchainAddr := GetArkAddress(text)
	return len(invoice)+len(onchainAddr)+len(offchainAddr) > 0
}

func GetArkAddress(bip21 string) string {
	aux := strings.Split(bip21, "?")
	if len(aux) < 2 {
		return ""
	}
	params := strings.Split(aux[1], "&")
	for _, param := range params {
		if kv := strings.Split(param, "="); len(kv) > 0 {
			if kv[0] == "ark" {
				if IsValidArkAddress(kv[1]) {
					return kv[1]
				}
			}
		}
	}
	return ""
}

func GetBtcAddress(bip21 string) string {
	aux := strings.Split(bip21, "?")
	if startsWithBitcoinPrefix(aux[0]) {
		if xua := strings.Split(aux[0], ":"); len(xua) > 1 {
			if IsValidBtcAddress(xua[1]) {
				return xua[1]
			}
		}
	}
	return ""
}

func GetInvoice(bip21 string) string {
	aux := strings.Split(bip21, "?")
	if len(aux) < 2 {
		return ""
	}
	params := strings.Split(aux[1], "&")
	for _, param := range params {
		if kv := strings.Split(param, "="); len(kv) > 0 {
			if kv[0] == "lightning" {
				if IsValidInvoice(kv[1]) {
					return kv[1]
				}
			}
		}
	}
	return ""
}

func SatsFromBip21(bip21 string) int {
	if !IsBip21(bip21) {
		return 0
	}
	aux := strings.Split(bip21, "?")
	if len(aux) < 2 {
		return 0
	}
	params := strings.Split(aux[1], "&")
	for _, param := range params {
		if kv := strings.Split(param, "="); len(kv) > 0 {
			if kv[0] == "amount" {
				if amount, err := strconv.Atoi(kv[1]); err == nil {
					return int(amount * 100000000)
				}
			}
		}
	}
	return 0
}

func startsWithBitcoinPrefix(s string) bool {
	return len(s) >= 8 && s[:8] == "bitcoin:"
}

func IsValidArkAddress(address string) bool {
	var re = regexp.MustCompile(`^(tark|ark)[a-zA-Z0-9]{110,118}$`)
	return re.MatchString(address)
}

func IsValidBtcAddress(address string) bool {
	var re = regexp.MustCompile(`^(bc|tb|[13])[a-zA-Z0-9]{25,62}$`)
	return re.MatchString(address)
}
