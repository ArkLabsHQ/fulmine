package domain

import "context"

// VtxoRolloverTarget represents an address being watched for rollover
type VtxoRolloverTarget struct {
	Address            string   // The address being watched
	TaprootTree        []string // Full taproot tree as list of hex tapscripts
	DestinationAddress string   // Where VTXO should be rolled over to
}

type VtxoRolloverRepository interface {
	AddTarget(ctx context.Context, target VtxoRolloverTarget) error
	GetTarget(ctx context.Context, address string) (*VtxoRolloverTarget, error)
	GetAllTargets(ctx context.Context) ([]VtxoRolloverTarget, error)
	RemoveTarget(ctx context.Context, address string) error
}
