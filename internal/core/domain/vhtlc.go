package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
)

type Vhtlc struct {
	vhtlc.Opts
	Id string
	// ExtraPacket contains the encrypted preimage in case of non interactive claim enabled. 
	
	ExtraPacket []byte
}

// VHTLCRepository stores the VHTLC options owned by the wallet
type VHTLCRepository interface {
	GetAll(ctx context.Context) ([]Vhtlc, error)
	Get(ctx context.Context, id string) (*Vhtlc, error)
	GetByIds(ctx context.Context, ids []string) ([]Vhtlc, error)
	Add(ctx context.Context, vhtlc Vhtlc) error
	Close()
}

func NewVhtlc(opts vhtlc.Opts, extraPacket []byte) Vhtlc {
	preimageHash := opts.PreimageHash
	sender := opts.Sender.SerializeCompressed()
	receiver := opts.Receiver.SerializeCompressed()
	return Vhtlc{
		Opts:        opts,
		Id:          GetVhtlcId(preimageHash, sender, receiver),
		ExtraPacket: extraPacket,
	}
}

func GetVhtlcId(preimageHash, sender, receiver []byte) string {
	id := make([]byte, 0, len(preimageHash)+len(sender)+len(receiver))
	id = append(id, preimageHash...)
	id = append(id, sender...)
	id = append(id, receiver...)
	id_hash := sha256.Sum256(id)
	return hex.EncodeToString(id_hash[:])
}
