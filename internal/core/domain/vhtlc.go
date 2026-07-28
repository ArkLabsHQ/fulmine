package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

type Vhtlc struct {
	Id     string
	Script string
}

type LegacyVhtlc struct {
	PreimageHash   string
	Sender         string
	Receiver       string
	Server         string
	RefundLocktime int64

	UnilateralClaimDelayType  int64
	UnilateralClaimDelayValue int64

	UnilateralRefundDelayType  int64
	UnilateralRefundDelayValue int64

	UnilateralRefundWithoutReceiverDelayType  int64
	UnilateralRefundWithoutReceiverDelayValue int64
}

// VHTLCRepository stores the VHTLC options owned by the wallet
type VHTLCRepository interface {
	GetAll(ctx context.Context) ([]Vhtlc, error)
	Get(ctx context.Context, id string) (*Vhtlc, error)
	GetByIds(ctx context.Context, ids []string) ([]Vhtlc, error)
	Add(ctx context.Context, vhtlc Vhtlc) error

	// Legacy single-key upgrade support.
	HasLegacy(ctx context.Context) (bool, error)
	GetLegacy(ctx context.Context) ([]LegacyVhtlc, error)
	DropLegacy(ctx context.Context) error

	Close()
}

func NewVhtlc(vhtlcId, script string) Vhtlc {
	return Vhtlc{
		Id:     vhtlcId,
		Script: script,
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
