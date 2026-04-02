package banco

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/introspector/pkg/arkade"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/asset"
	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript"
)

// PacketType is the extension packet type tag for a banco swap offer (0x03).
const PacketType = uint8(0x03)

const (
	tlvSwapPkScript        = 0x01
	tlvWantAmount          = 0x02
	tlvWantAsset           = 0x03
	tlvCancelDelay         = 0x04
	tlvMakerPkScript       = 0x05
	tlvMakerPublicKey      = 0x07
	tlvIntrospectorPubkey  = 0x08
	tlvExitTimelock        = 0x0c
)

// BancoOffer contains all the fields from a decoded banco swap offer TLV packet.
type BancoOffer struct {
	SwapPkScript       []byte
	WantAmount         uint64
	WantAsset          *asset.AssetId      // nil = BTC
	CancelAt           uint64              // unix timestamp; 0 = no cancel path
	MakerPkScript      []byte              // 34 bytes: OP_1 + PUSH32 + 32-byte key
	MakerPublicKey     *btcec.PublicKey     // required if cancel or exit
	IntrospectorPubkey *btcec.PublicKey     // required; x-only 32 bytes
	ExitDelay          *arklib.RelativeLocktime
}

// DeserializeOffer parses the TLV payload from an UnknownPacket whose type == PacketType.
// TLV format: [type: 1B][length: 2B big-endian][value bytes]
func DeserializeOffer(data []byte) (*BancoOffer, error) {
	offer := &BancoOffer{}

	hasSwapPkScript := false
	hasWantAmount := false
	hasMakerPkScript := false
	hasIntrospectorPubkey := false

	offset := 0
	for offset < len(data) {
		// Need at least 3 bytes for type + length header
		if offset+3 > len(data) {
			return nil, errors.New("truncated TLV: not enough bytes for type+length header")
		}

		tlvType := data[offset]
		tlvLength := int(binary.BigEndian.Uint16(data[offset+1 : offset+3]))
		offset += 3

		if offset+tlvLength > len(data) {
			return nil, fmt.Errorf(
				"truncated TLV: expected %d bytes for type 0x%02x, got %d",
				tlvLength, tlvType, len(data)-offset,
			)
		}

		value := make([]byte, tlvLength)
		copy(value, data[offset:offset+tlvLength])
		offset += tlvLength

		switch tlvType {
		case tlvSwapPkScript:
			offer.SwapPkScript = value
			hasSwapPkScript = true
		case tlvWantAmount:
			if len(value) != 8 {
				return nil, fmt.Errorf("invalid wantAmount: expected 8 bytes, got %d", len(value))
			}
			offer.WantAmount = binary.BigEndian.Uint64(value)
			hasWantAmount = true
		case tlvWantAsset:
			assetId, err := asset.NewAssetIdFromBytes(value)
			if err != nil {
				return nil, fmt.Errorf("invalid wantAsset: %w", err)
			}
			offer.WantAsset = assetId
		case tlvCancelDelay:
			if len(value) != 8 {
				return nil, fmt.Errorf("invalid cancelDelay: expected 8 bytes, got %d", len(value))
			}
			offer.CancelAt = binary.BigEndian.Uint64(value)
		case tlvMakerPkScript:
			offer.MakerPkScript = value
			hasMakerPkScript = true
		case tlvMakerPublicKey:
			pubkey, err := schnorr.ParsePubKey(value)
			if err != nil {
				return nil, fmt.Errorf("invalid maker public key: %w", err)
			}
			offer.MakerPublicKey = pubkey
		case tlvIntrospectorPubkey:
			if len(value) != 32 {
				return nil, fmt.Errorf("invalid introspectorPubkey: expected 32 bytes, got %d", len(value))
			}
			pubkey, err := schnorr.ParsePubKey(value)
			if err != nil {
				return nil, fmt.Errorf("invalid introspector public key: %w", err)
			}
			offer.IntrospectorPubkey = pubkey
			hasIntrospectorPubkey = true
		case tlvExitTimelock:
			if len(value) != 9 {
				return nil, fmt.Errorf("invalid exitTimelock: expected 9 bytes, got %d", len(value))
			}
			typeExit := arklib.LocktimeTypeBlock
			if value[0] == 1 {
				typeExit = arklib.LocktimeTypeSecond
			}
			delay := binary.BigEndian.Uint64(value[1:9])
			offer.ExitDelay = &arklib.RelativeLocktime{
				Type:  typeExit,
				Value: uint32(delay),
			}
		default:
			return nil, fmt.Errorf("unknown TLV type: 0x%02x", tlvType)
		}
	}

	// Validate required fields
	if !hasSwapPkScript {
		return nil, errors.New("missing required field: swapPkScript")
	}
	if !hasWantAmount {
		return nil, errors.New("missing required field: wantAmount")
	}
	if !hasMakerPkScript {
		return nil, errors.New("missing required field: makerPkScript")
	}
	if !hasIntrospectorPubkey {
		return nil, errors.New("missing required field: introspectorPubkey")
	}

	if len(offer.MakerPkScript) != 34 {
		return nil, fmt.Errorf("invalid makerPkScript: expected 34 bytes, got %d", len(offer.MakerPkScript))
	}

	return offer, nil
}

// Serialize encodes a BancoOffer into TLV bytes.
// Matches ts-sdk Offer.encode().
func (o *BancoOffer) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	encodeTLV(&buf, tlvSwapPkScript, o.SwapPkScript)

	amountBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amountBuf, o.WantAmount)
	encodeTLV(&buf, tlvWantAmount, amountBuf)

	if o.WantAsset != nil {
		assetBytes, err := o.WantAsset.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize wantAsset: %w", err)
		}
		encodeTLV(&buf, tlvWantAsset, assetBytes)
	}

	if o.CancelAt > 0 {
		delayBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(delayBuf, o.CancelAt)
		encodeTLV(&buf, tlvCancelDelay, delayBuf)
	}

	encodeTLV(&buf, tlvMakerPkScript, o.MakerPkScript)
	if o.MakerPublicKey != nil {
		encodeTLV(&buf, tlvMakerPublicKey, schnorr.SerializePubKey(o.MakerPublicKey))
	}
	if o.IntrospectorPubkey != nil {
		encodeTLV(&buf, tlvIntrospectorPubkey, schnorr.SerializePubKey(o.IntrospectorPubkey))
	}

	if o.ExitDelay != nil {
		exitBuf := make([]byte, 9)
		if o.ExitDelay.Type == arklib.LocktimeTypeSecond {
			exitBuf[0] = 1
		}
		binary.BigEndian.PutUint64(exitBuf[1:], uint64(o.ExitDelay.Value))
		encodeTLV(&buf, tlvExitTimelock, exitBuf)
	}

	return buf.Bytes(), nil
}

// ToPacket wraps an offer as an extension packet.
func (o *BancoOffer) ToPacket() (extension.Packet, error) {
	data, err := o.Serialize()
	if err != nil {
		return nil, err
	}
	return extension.UnknownPacket{PacketType: PacketType, Data: data}, nil
}

// FulfillScript returns the arkade script for the fulfill path.
// For BTC swaps it checks output[0] pays wantAmount to makerPkScript.
// For asset swaps it uses INSPECTOUTASSETLOOKUP.
func (o *BancoOffer) FulfillScript() ([]byte, error) {
	makerWitnessProgram := o.MakerPkScript[2:]

	scriptPubKeyCheck := func(b *txscript.ScriptBuilder) {
		b.AddOp(txscript.OP_0)
		b.AddOp(script.OP_INSPECTOUTPUTSCRIPTPUBKEY)
		b.AddOp(txscript.OP_1) // version 1 (taproot)
		b.AddOp(txscript.OP_EQUALVERIFY)
		b.AddData(makerWitnessProgram)
		b.AddOp(txscript.OP_EQUAL)
	}

	if o.WantAsset == nil {
		// BTC fulfill: value check + scriptPubKey check
		b := txscript.NewScriptBuilder()
		b.AddOp(txscript.OP_0)
		b.AddOp(script.OP_INSPECTOUTPUTVALUE)
		b.AddInt64(int64(o.WantAmount))
		b.AddOp(arkade.OP_SCRIPTNUMTOLE64)
		b.AddOp(arkade.OP_GREATERTHANOREQUAL64)
		b.AddOp(txscript.OP_VERIFY)
		scriptPubKeyCheck(b)
		return b.Script()
	}

	// Asset fulfill: INSPECTOUTASSETLOOKUP + value check + scriptPubKey check
	// Stack for INSPECTOUTASSETLOOKUP (top-to-bottom): lookup_index, txid, output_index.
	// For full fill, the taker creates a single asset group at output 0, so lookup_index=0.
	b := txscript.NewScriptBuilder()
	b.AddOp(txscript.OP_0) // output index 0
	b.AddData(o.WantAsset.Txid[:])
	b.AddOp(txscript.OP_0) // asset group lookup index 0
	b.AddOp(arkade.OP_INSPECTOUTASSETLOOKUP)
	b.AddOp(txscript.OP_DUP)
	b.AddOp(txscript.OP_1NEGATE)
	b.AddOp(txscript.OP_EQUAL)
	b.AddOp(txscript.OP_NOT)
	b.AddOp(txscript.OP_VERIFY)
	b.AddOp(arkade.OP_SCRIPTNUMTOLE64)
	b.AddInt64(int64(o.WantAmount))
	b.AddOp(arkade.OP_SCRIPTNUMTOLE64)
	b.AddOp(arkade.OP_GREATERTHANOREQUAL64)
	b.AddOp(txscript.OP_VERIFY)
	scriptPubKeyCheck(b)
	return b.Script()
}

// VtxoScript builds the full VTXO taptree (fulfill + optional cancel + exit).
func (s *BancoOffer) VtxoScript(server *btcec.PublicKey) (*script.TapscriptsVtxoScript, error) {
	fulfillScript, err := s.FulfillScript()
	if err != nil {
		return nil, err
	}

	scriptHash := arkade.ArkadeScriptHash(fulfillScript)
	tweakedKey := arkade.ComputeArkadeScriptPublicKey(s.IntrospectorPubkey, scriptHash)

	closures := []script.Closure{&script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{server, tweakedKey},
	}}

	if s.CancelAt > 0 {
		cancelClosure := &script.CLTVMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{s.MakerPublicKey, server},
			},
			Locktime: arklib.AbsoluteLocktime(s.CancelAt),
		}
		closures = append(closures, cancelClosure)
	}

	if s.ExitDelay != nil {
		exitClosure := &script.CSVMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{s.MakerPublicKey, server},
			},
			Locktime: *s.ExitDelay,
		}
		closures = append(closures, exitClosure)
	}

	return &script.TapscriptsVtxoScript{Closures: closures}, nil
}

// FindBancoOffer searches extension packets for a banco offer.
// Returns nil, nil if not found (not an error -- the tx just isn't an offer).
func FindBancoOffer(ext extension.Extension) (*BancoOffer, error) {
	for _, p := range ext {
		unknown, ok := p.(extension.UnknownPacket)
		if !ok {
			continue
		}
		if unknown.PacketType != PacketType {
			continue
		}
		offer, err := DeserializeOffer(unknown.Data)
		if err != nil {
			return nil, fmt.Errorf("malformed banco offer packet: %w", err)
		}
		return offer, nil
	}
	return nil, nil
}

func encodeTLV(buf *bytes.Buffer, tlvType byte, value []byte) {
	buf.WriteByte(tlvType)
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(value)))
	buf.Write(lenBuf)
	buf.Write(value)
}