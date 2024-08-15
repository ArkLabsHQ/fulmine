package handlers

import (
	"fmt"
	"regexp"
	"strings"
)

func genBip21(offchainAddr, onchainAddr, sats string) string {
	bip21 := fmt.Sprintf("bitcoin:%s?ark=%s", onchainAddr, offchainAddr)
	// add amount if passed
	if sats != "" {
		amount := fmt.Sprintf("&amount=%s", sats)
		bip21 += amount
	}
	return bip21
}

func isBip21(invoice string) bool {
	if !startsWithBitcoinPrefix(invoice) {
		return false
	}
	address := getBtcAddress(invoice)
	return len(address) > 0
}

func getArkAddress(invoice string) string {
	aux := strings.Split(invoice, "?")
	if len(aux) < 2 {
		return ""
	}
	params := strings.Split(aux[1], "&")
	for _, param := range params {
		if kv := strings.Split(param, "="); len(kv) > 0 {
			if kv[0] == "ark" {
				if isValidArkAddress(kv[1]) {
					return kv[1]
				}
			}
		}
	}
	return ""
}

func getBtcAddress(invoice string) string {
	aux := strings.Split(invoice, "?")
	if startsWithBitcoinPrefix(aux[0]) {
		if xua := strings.Split(aux[0], ":"); len(xua) > 1 {
			if isValidBtcAddress(xua[1]) {
				return xua[1]
			}
		}
	}
	return ""
}

func startsWithBitcoinPrefix(s string) bool {
	return len(s) >= 8 && s[:8] == "bitcoin:"
}

func isValidArkAddress(address string) bool {
	var re = regexp.MustCompile(`^(tark|ark)[a-zA-Z0-9]{110,118}$`)
	return re.MatchString(address)
}

func isValidBtcAddress(address string) bool {
	var re = regexp.MustCompile(`^(bc|[13])[a-zA-Z0-9]{25,45}$`)
	return re.MatchString(address)
}
