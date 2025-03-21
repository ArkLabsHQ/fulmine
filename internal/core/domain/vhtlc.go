package domain

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
)

// VHTLCRepository stores the VHTLC options owned by the wallet
type VHTLCRepository interface {
	GetAll(ctx context.Context) ([]vhtlc.Opts, error)
	Get(ctx context.Context, preimageHash string) (*vhtlc.Opts, error)
	Add(ctx context.Context, opts vhtlc.Opts) error
	Delete(ctx context.Context, preimageHash string) error
}
