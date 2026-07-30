package utils

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
)

func IsBip21(text string) bool {
	if !startsWithBitcoinPrefix(text) {
		return false
	}
	onchainAddr := GetBtcAddress(text)
	offchainAddr := GetOffchainAddress(text)
	return len(onchainAddr)+len(offchainAddr) > 0
}

func GetOffchainAddress(bip21 string) string {
	aux := strings.Split(bip21, "?")
	if len(aux) < 2 {
		return ""
	}
	params := strings.SplitSeq(aux[1], "&")
	for param := range params {
		// SplitN with a limit of 2 keeps any "=" inside the value, and the
		// len > 1 check is load-bearing: a valueless parameter such as
		// "bitcoin:<addr>?ark" splits into one element, and indexing kv[1]
		// would panic on input that reaches here straight from the send form.
		if kv := strings.SplitN(param, "=", 2); len(kv) > 1 {
			if kv[0] == "ark" {
				if IsValidOffchainAddress(kv[1]) {
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

// SatsFromBip21 returns the amount carried by a bip21 URI in satoshis, or 0 if
// it carries none.
//
// BIP21 denominates amount in decimal BTC, which is what Service.NewAddress
// emits (`fmt.Sprintf("%.8f", btc)` -> "?amount=0.00001000").
func SatsFromBip21(bip21 string) uint64 {
	if !IsBip21(bip21) {
		return 0
	}
	aux := strings.Split(bip21, "?")
	if len(aux) < 2 {
		return 0
	}
	params := strings.Split(aux[1], "&")
	for _, param := range params {
		// Same guard as GetOffchainAddress: "?amount" with no value would
		// otherwise panic on kv[1].
		if kv := strings.SplitN(param, "=", 2); len(kv) > 1 {
			if kv[0] == "amount" {
				btc, err := strconv.ParseFloat(kv[1], 64)
				if err != nil {
					return 0
				}
				sats, err := btcutil.NewAmount(btc)
				if err != nil || sats <= 0 {
					return 0
				}
				return uint64(sats)
			}
		}
	}
	return 0
}

func startsWithBitcoinPrefix(s string) bool {
	return len(s) >= 8 && s[:8] == "bitcoin:"
}

func IsValidOffchainAddress(address string) bool {
	var re = regexp.MustCompile(`^(tark|ark)[a-zA-Z0-9]{110,118}$`)
	return re.MatchString(address)
}

func IsValidBtcAddress(address string) bool {
	var re = regexp.MustCompile(`^(bc|tb|[13])[a-zA-Z0-9]{25,62}$`)
	return re.MatchString(address)
}
