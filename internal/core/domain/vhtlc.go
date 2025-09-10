package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
)

// VHTLCRepository stores the VHTLC options owned by the wallet
type VHTLCRepository interface {
	GetAll(ctx context.Context) ([]vhtlc.Opts, error)
	Get(ctx context.Context, id string) (*vhtlc.Opts, error)
	Add(ctx context.Context, opts vhtlc.Opts) error
	Close()
}

func CreateVhtlcId(preimageHash, sender, receiver []byte) string {
	id := make([]byte, 0, len(preimageHash)+len(sender)+len(receiver))
	id = append(id, preimageHash...)
	id = append(id, sender...)
	id = append(id, receiver...)
	id_hash := sha256.Sum256(id)
	return hex.EncodeToString(id_hash[:])
}
