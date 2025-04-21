package utils

import (
	"regexp"
	"strings"
)

func IsBip21(invoice string) bool {
	if !startsWithBitcoinPrefix(invoice) {
		return false
	}
	onchainAddr := GetBtcAddress(invoice)
	offchainAddr := GetArkAddress(invoice)
	return len(onchainAddr)+len(offchainAddr) > 0
}

func GetArkAddress(invoice string) string {
	aux := strings.Split(invoice, "?")
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

func GetBtcAddress(invoice string) string {
	aux := strings.Split(invoice, "?")
	if startsWithBitcoinPrefix(aux[0]) {
		if xua := strings.Split(aux[0], ":"); len(xua) > 1 {
			if IsValidBtcAddress(xua[1]) {
				return xua[1]
			}
		}
	}
	return ""
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
