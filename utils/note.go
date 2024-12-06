package utils

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil/base58"
)

const noteHRP = "arknote"

// Note represents a note signed by the issuer
type Note struct {
	Data
	Signature []byte
}

// Data contains the data of a note
type Data struct {
	ID    uint64
	Value uint32
}

// NewFromString converts a base58 encoded string with HRP to a Note
func NoteFromString(s string) (*Note, error) {
	if !strings.HasPrefix(s, noteHRP) {
		return nil, fmt.Errorf("invalid human-readable part: expected %s prefix (note '%s')", noteHRP, s)
	}

	encoded := strings.TrimPrefix(s, noteHRP)
	decoded := base58.Decode(encoded)
	if len(decoded) == 0 {
		return nil, fmt.Errorf("failed to decode base58 string")
	}

	note := &Note{}
	err := note.Deserialize(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize note: %w", err)
	}

	return note, nil
}

// Deserialize converts a byte slice to Data
func (n *Data) Deserialize(data []byte) error {
	if len(data) != 12 {
		return fmt.Errorf("invalid data length: expected 12 bytes, got %d", len(data))
	}

	n.ID = binary.BigEndian.Uint64(data[:8])
	n.Value = binary.BigEndian.Uint32(data[8:])
	return nil
}

// Deserialize converts a byte slice to a Note
func (n *Note) Deserialize(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("invalid data length: expected at least 12 bytes, got %d", len(data))
	}

	dataCopy := &Data{}
	if err := dataCopy.Deserialize(data[:12]); err != nil {
		return err
	}

	n.Data = *dataCopy

	if len(data) > 12 {
		n.Signature = data[12:]
	}

	return nil
}

func SatsFromNote(text string) int {
	note, err := NoteFromString(text)
	if err != nil {
		return 0
	}
	return int(note.Value)
}

func IsValidArkNote(text string) bool {
	return SatsFromNote(text) > 0
}
