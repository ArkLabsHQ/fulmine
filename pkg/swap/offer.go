package swap

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/lightningnetwork/lnd/tlv"
)

type OfferData struct {
	Currency       string // TLV type 6
	AmountMillisat uint64 // TLV type 8
	AbsoluteExpiry uint32 // TLV type 14
	Description    string // TLV type 10
}

func Decode(offer string) (*OfferData, error) {
	// --- 1) Bech32 decode to get raw TLV payload ---
	hrp, data, err := bech32.DecodeNoLimit(offer)
	if err != nil {
		return nil, fmt.Errorf("bech32.Decode: %w", err)
	}
	if hrp != "lno" {
		return nil, errors.New("invalid BOLT12 human-readable part")
	}
	payload, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return nil, fmt.Errorf("ConvertBits: %w", err)
	}

	out := &OfferData{}
	records := []tlv.Record{
		tlv.MakePrimitiveRecord(6, &out.Currency),
		tlv.MakePrimitiveRecord(8, &out.AmountMillisat),
		tlv.MakePrimitiveRecord(14, &out.AbsoluteExpiry),
		tlv.MakePrimitiveRecord(10, &out.Description),
	}
	stream, err := tlv.NewStream(records...)
	if err != nil {
		return nil, fmt.Errorf("tlv.NewStream: %w", err)
	}
	if err := stream.Decode(bytes.NewReader(payload)); err != nil {
		return nil, fmt.Errorf("TLV decode: %w", err)
	}

	return out, nil
}
