package utils

import (
	"github.com/ark-network/ark/common/note"
)

func SatsFromNote(text string) int {
	n, err := note.NewFromString(text)
	if err != nil {
		return 0
	}
	return int(n.Value)
}

func IsValidArkNote(text string) bool {
	return SatsFromNote(text) > 0
}
