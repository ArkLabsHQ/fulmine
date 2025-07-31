package swap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/ccoveille/go-safecast"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/tlv"
)

type Offer struct {
	ID             string
	Amount         uint64
	AmountInSats   uint64
	Description    []byte
	DescriptionStr string
	AbsoluteExpiry uint64
	QuantityMax    uint64
}

type Invoice struct {
	Amount         uint64
	AmountInSats   uint64
	PaymentHash    []byte
	PaymentHash160 []byte
}

// BOLT12 TLV types as Go constants
const (
	OFFER_CHAINS          uint64 = 2
	OFFER_METADATA        uint64 = 4
	OFFER_CURRENCY        uint64 = 6
	OFFER_AMOUNT          uint64 = 8
	OFFER_DESCRIPTION     uint64 = 10
	OFFER_FEATURES        uint64 = 12
	OFFER_ABSOLUTE_EXPIRY uint64 = 14
	OFFER_PATHS           uint64 = 16
	OFFER_ISSUER          uint64 = 18
	OFFER_QUANTITY_MAX    uint64 = 20
	OFFER_ISSUER_ID       uint64 = 22

	INVOICE_PAYMENT_HASH uint64 = 168
	INVOICE_AMOUNT       uint64 = 170
)

func DecodeBolt12Invoice(invoice string) (*Invoice, error) {
	hrp, words, err := bech32ToWords(invoice)
	if err != nil {
		return nil, fmt.Errorf("bech32ToWords: %w", err)
	}

	if hrp != "lni" {
		return nil, errors.New("invalid BOLT12 human-readable part")
	}

	payload, err := bech32.ConvertBits(words, 5, 8, false)
	if err != nil {
		return nil, fmt.Errorf("ConvertBits: %w", err)
	}

	invoiceData := new(Invoice)

	sizeFunc := func() uint64 {
		return tlv.SizeTUint64(invoiceData.Amount)
	}

	tlvStream := tlv.MustNewStream(
		tlv.MakePrimitiveRecord(tlv.Type(INVOICE_PAYMENT_HASH), &invoiceData.PaymentHash),
		tlv.MakeDynamicRecord(tlv.Type(INVOICE_AMOUNT), &invoiceData.Amount, sizeFunc, tlv.ETUint64, tlv.DTUint64),
	)

	err = tlvStream.Decode(bytes.NewReader(payload))

	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	invoiceData.AmountInSats, err = safecast.ToUint64(invoiceData.Amount / 1000)
	if err != nil {
		return nil, fmt.Errorf("invalid amount in sats: %w", err)
	}

	invoiceData.PaymentHash160 = input.Ripemd160H(invoiceData.PaymentHash)

	return invoiceData, nil
}

func DecodeBolt12Offer(offer string) (*Offer, error) {
	hrp, words, err := bech32ToWords(offer)
	if err != nil {
		return nil, fmt.Errorf("bech32ToWords: %w", err)
	}

	if hrp != "lno" {
		return nil, errors.New("invalid BOLT12 human-readable part")
	}

	payload, err := bech32.ConvertBits(words, 5, 8, false)
	if err != nil {
		return nil, fmt.Errorf("ConvertBits: %w", err)
	}

	offerData := new(Offer)

	sizeFunc := func() uint64 {
		return tlv.SizeTUint64(offerData.Amount)
	}

	tlvStream := tlv.MustNewStream(
		tlv.MakeDynamicRecord(tlv.Type(OFFER_AMOUNT), &offerData.Amount, sizeFunc, tlv.ETUint64, tlv.DTUint64),
		tlv.MakePrimitiveRecord(tlv.Type(OFFER_DESCRIPTION), &offerData.Description),
		tlv.MakeDynamicRecord(tlv.Type(OFFER_ABSOLUTE_EXPIRY), &offerData.AbsoluteExpiry, sizeFunc, tlv.ETUint64, tlv.DTUint64),
		tlv.MakePrimitiveRecord(tlv.Type(OFFER_QUANTITY_MAX), &offerData.QuantityMax),
	)

	err = tlvStream.Decode(bytes.NewReader(payload))

	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	offerData.AmountInSats, err = safecast.ToUint64(offerData.Amount / 1000)
	if err != nil {
		return nil, fmt.Errorf("invalid amount in sats: %w", err)
	}

	offerData.DescriptionStr = string(offerData.Description)

	offerHash := sha256.Sum256(payload)

	offerData.ID = hex.EncodeToString(offerHash[:])

	return offerData, nil
}

// Remove formatting characters and return cleaned string
func cleanBech32String(bech32String string) string {
	// Remove "+" followed by any whitespace, globally
	var cleaned strings.Builder
	skip := false
	for _, r := range bech32String {
		if r == '+' {
			skip = true
			continue
		}
		if skip && unicode.IsSpace(r) {
			continue
		}
		skip = false
		cleaned.WriteRune(r)
	}
	return cleaned.String()
}

// Parse and decode Bech32 data part into 5-bit words
func bech32ToWords(bech32String string) (prefix string, words []byte, err error) {
	// 1. Remove formatting and make lowercase
	cleanString := strings.ToLower(cleanBech32String(bech32String))

	// 2. Split into prefix and data by first '1'
	parts := strings.SplitN(cleanString, "1", 2)
	if len(parts) != 2 {
		return "", nil, errors.New("invalid BOLT12 format: missing separator")
	}
	prefix, data := parts[0], parts[1]

	// 3. Bech32 charset
	const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

	// 4. Convert data chars to 5-bit words
	words = make([]byte, 0, len(data))
	for _, r := range data {
		index := strings.IndexRune(charset, r)
		if index == -1 {
			return "", nil, errors.New("invalid character in data part")
		}
		words = append(words, byte(index))
	}

	return prefix, words, nil
}

func IsBolt12Offer(offer string) bool {
	// BOLT12 offers start with "lno" prefix
	return strings.HasPrefix(strings.ToLower(offer), "lno")
}

func SatsFromBolt12Offer(offer string) int {
	decodedOffer, err := DecodeBolt12Offer(offer)
	if err != nil {
		return 0
	}
	return int(decodedOffer.Amount)
}

func IsValidBolt12Offer(offer string) bool {
	return SatsFromBolt12Offer(offer) > 0
}

func IsBolt12Invoice(invoice string) bool {
	return strings.HasPrefix(strings.ToLower(invoice), "lni")
}
