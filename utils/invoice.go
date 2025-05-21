package utils

import decodepay "github.com/nbd-wtf/ln-decodepay"

func DecodeInvoice(invoice string) (uint64, []byte, error) {
	bolt11, err := decodepay.Decodepay(invoice)
	if err != nil {
		return 0, []byte{}, err
	}
	amount := uint64(bolt11.MSatoshi / 1000)
	preimageHash := []byte(bolt11.PaymentHash)
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
