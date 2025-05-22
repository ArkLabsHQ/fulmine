package utils

import (
	"encoding/hex"

	decodepay "github.com/nbd-wtf/ln-decodepay"
)

func DecodeInvoice(invoice string) (uint64, []byte, error) {
	bolt11, err := decodepay.Decodepay(invoice)
	if err != nil {
		return 0, nil, err
	}

	amount := uint64(bolt11.MSatoshi / 1000)
	preimageHash, err := hex.DecodeString(bolt11.PaymentHash)
	if err != nil {
		return 0, nil, err
	}

	return amount, preimageHash, nil
}

func SatsFromInvoice(invoice string) int {
	n, err := decodepay.Decodepay(invoice)
	if err != nil {
		return 0
	}
	return int(n.MSatoshi / 1000)
}

func IsValidInvoice(invoice string) bool {
	return SatsFromInvoice(invoice) > 0
}
