package swap

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"

	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/ccoveille/go-safecast"
	"github.com/lightningnetwork/lnd/input"
)

type Offer struct {
	ID             string
	Type           string
	Currency       string
	Amount         uint64
	Description    string
	AbsoluteExpiry uint64
	QuantityMax    uint64
}

type Invoice struct {
	Amount      uint64
	PaymentHash []byte
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

// parseTruncatedUint parses a variable-length unsigned big-endian integer (up to 8 bytes)
func parseTruncatedUint(buffer []byte) (uint64, error) {
	if len(buffer) > 8 {
		return 0, errors.New("invalid truncated uint length")
	}

	value := big.NewInt(0)
	for _, b := range buffer {
		value.Lsh(value, 8) // value = value << 8
		value.Or(value, big.NewInt(int64(b)))
	}
	return value.Uint64(), nil
}

// TLVParsingError for parsing failures
type TLVParsingError struct {
	Message string
}

func (e TLVParsingError) Error() string {
	return e.Message
}

// TLVRecord for a parsed TLV record
type TLVRecord struct {
	Type   *big.Int
	Length *big.Int
	Value  []byte
}

// TLVParser for parsing TLV streams
type TLVParser struct {
	position int
	buffer   []byte
}

// NewTLVParser constructs a TLVParser
func NewTLVParser(data []byte) *TLVParser {
	return &TLVParser{position: 0, buffer: data}
}

// ParseBigSize parses a BigSize as specified in the Lightning spec
func (p *TLVParser) ParseBigSize() (*big.Int, error) {
	if p.position >= len(p.buffer) {
		return nil, TLVParsingError{"Unexpected end of data while parsing BigSize"}
	}

	first := p.buffer[p.position]

	// Single byte encoding (0-252)
	if first <= 252 {
		p.position++
		return big.NewInt(int64(first)), nil
	}

	// Two byte encoding (253 + uint16)
	if first == 253 {
		if p.position+3 > len(p.buffer) {
			return nil, TLVParsingError{"Insufficient bytes for 2-byte BigSize"}
		}
		value := binary.BigEndian.Uint16(p.buffer[p.position+1 : p.position+3])
		if value <= 252 {
			return nil, TLVParsingError{"Non-minimal BigSize encoding"}
		}
		p.position += 3
		return big.NewInt(int64(value)), nil
	}

	// Four byte encoding (254 + uint32)
	if first == 254 {
		if p.position+5 > len(p.buffer) {
			return nil, TLVParsingError{"Insufficient bytes for 4-byte BigSize"}
		}
		value := binary.BigEndian.Uint32(p.buffer[p.position+1 : p.position+5])
		if value <= 0xffff {
			return nil, TLVParsingError{"Non-minimal BigSize encoding"}
		}
		p.position += 5
		return big.NewInt(int64(value)), nil
	}

	// Eight byte encoding (255 + uint64)
	if first == 255 {
		if p.position+9 > len(p.buffer) {
			return nil, TLVParsingError{"Insufficient bytes for 8-byte BigSize"}
		}
		value := binary.BigEndian.Uint64(p.buffer[p.position+1 : p.position+9])
		if value <= 0xffffffff {
			return nil, TLVParsingError{"Non-minimal BigSize encoding"}
		}
		p.position += 9
		return new(big.Int).SetUint64(value), nil
	}

	return nil, TLVParsingError{"Invalid BigSize encoding"}
}

// ParseTLVRecord parses a single TLV record
func (p *TLVParser) ParseTLVRecord() (*TLVRecord, error) {
	// Parse type
	t, err := p.ParseBigSize()
	if err != nil {
		return nil, err
	}

	// Parse length
	l, err := p.ParseBigSize()
	if err != nil {
		return nil, err
	}

	llen := int(l.Int64())
	if p.position+llen > len(p.buffer) {
		return nil, TLVParsingError{"Insufficient bytes for TLV value"}
	}

	val := make([]byte, llen)
	copy(val, p.buffer[p.position:p.position+llen])
	p.position += llen

	return &TLVRecord{
		Type:   t,
		Length: l,
		Value:  val,
	}, nil
}

// ParseTLVStream parses the entire TLV stream into records
func (p *TLVParser) ParseTLVStream() ([]*TLVRecord, error) {
	var records []*TLVRecord
	lastType := big.NewInt(-1)

	for p.position < len(p.buffer) {
		record, err := p.ParseTLVRecord()
		if err != nil {
			return nil, err
		}
		// Check for strictly increasing type values
		if record.Type.Cmp(lastType) <= 0 {
			return nil, TLVParsingError{"TLV records not in strictly-increasing order"}
		}
		// Handle unknown even-typed records >= 65536
		if record.Type.Cmp(big.NewInt(65536)) >= 0 && new(big.Int).Mod(record.Type, big.NewInt(2)).Cmp(big.NewInt(0)) == 0 {
			return nil, TLVParsingError{"Unknown even-typed record encountered"}
		}

		records = append(records, record)
		lastType = record.Type
	}
	return records, nil
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

	parser := NewTLVParser(payload)
	records, err := parser.ParseTLVStream()
	if err != nil {
		return nil, fmt.Errorf("ParseTLVStream: %w", err)
	}

	invData := Invoice{}

	for _, record := range records {
		switch record.Type.Uint64() {
		case INVOICE_PAYMENT_HASH:
			invData.PaymentHash = input.Ripemd160H(record.Value)
		case INVOICE_AMOUNT:
			milliSatsAmount, err := parseTruncatedUint(record.Value)
			if err != nil {
				return nil, err
			}
			invData.Amount, err = safecast.ToUint64(milliSatsAmount / 1000)
			if err != nil {
				return nil, fmt.Errorf("invalid amount: %w", err)
			}
		default:
			continue
		}
	}

	return &invData, nil
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

	parser := NewTLVParser(payload)
	records, err := parser.ParseTLVStream()
	if err != nil {
		return nil, fmt.Errorf("ParseTLVStream: %w", err)
	}

	offerData := Offer{ID: "", Type: "offer"}

	for _, record := range records {
		switch record.Type.Uint64() {
		case OFFER_CURRENCY:
			offerData.Currency = string(record.Value)
		case OFFER_AMOUNT:
			milliSatsAmount, err := parseTruncatedUint(record.Value)
			if err != nil {
				return nil, err
			}
			offerData.Amount, err = safecast.ToUint64(milliSatsAmount / 1000)
			if err != nil {
				return nil, fmt.Errorf("invalid amount: %w", err)
			}
		case OFFER_DESCRIPTION:
			offerData.Description = string(record.Value)
		case OFFER_ABSOLUTE_EXPIRY:
			offerData.AbsoluteExpiry, err = parseTruncatedUint(record.Value)
			if err != nil {
				return nil, err
			}
		case OFFER_QUANTITY_MAX:
			offerData.QuantityMax, err = parseTruncatedUint(record.Value)
			if err != nil {
				return nil, err
			}

		default:
			continue
		}

	}

	offerData.ID = serializeTLVRecords(records)

	return &offerData, nil
}

func serializeTLVRecords(records []*TLVRecord) string {
	// 1. Calculate total length
	totalLength := 0
	serializedRecords := make([][]byte, len(records))
	for i, record := range records {
		serialized, _ := serializeTLVRecord(record)
		serializedRecords[i] = serialized
		totalLength += len(serialized)
	}

	// 2. Serialize all records into a single buffer
	result := make([]byte, totalLength)
	offset := 0
	for _, serialized := range serializedRecords {
		copy(result[offset:], serialized)
		offset += len(serialized)
	}

	// 3. Compute SHA256 and hex-encode
	hash := sha256.Sum256(result)
	return hex.EncodeToString(hash[:])
}

// serializeTLVRecord encodes a TLVRecord into bytes with minimal BigSize for type and length
func serializeTLVRecord(record *TLVRecord) ([]byte, error) {
	// Validate that type and length are non-negative
	if record.Type.Sign() < 0 || record.Length.Sign() < 0 {
		return nil, errors.New("type and length must be non-negative")
	}

	if len(record.Value) != int(record.Length.Int64()) {
		return nil, errors.New("value length does not match length field")
	}

	// Encode type and length using BigSize
	typeBuf := encodeBigSize(record.Type)
	lenBuf := encodeBigSize(record.Length)

	// Concatenate all parts
	result := make([]byte, len(typeBuf)+len(lenBuf)+len(record.Value))
	offset := 0
	copy(result[offset:], typeBuf)
	offset += len(typeBuf)
	copy(result[offset:], lenBuf)
	offset += len(lenBuf)
	copy(result[offset:], record.Value)

	return result, nil
}

func encodeBigSize(val *big.Int) []byte {
	u := val.Uint64()
	switch {
	case u < 0xfd:
		return []byte{byte(u)}
	case u < 0x10000:
		// 0xfd + 2 bytes big-endian
		return []byte{
			0xfd,
			byte(u >> 8), byte(u),
		}
	case u < 0x100000000:
		// 0xfe + 4 bytes big-endian
		return []byte{
			0xfe,
			byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u),
		}
	default:
		// 0xff + 8 bytes big-endian
		return []byte{
			0xff,
			byte(u >> 56), byte(u >> 48), byte(u >> 40), byte(u >> 32),
			byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u),
		}
	}
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
