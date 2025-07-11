package utils

import (
	"github.com/arkade-os/arkd/pkg/ark-lib/note"
)

func SatsFromNote(text string) int {
	n, err := note.NewNoteFromString(text)
	if err != nil {
		return 0
	}
	return int(n.Value)
}

func IsValidArkNote(text string) bool {
	return SatsFromNote(text) > 0
}
