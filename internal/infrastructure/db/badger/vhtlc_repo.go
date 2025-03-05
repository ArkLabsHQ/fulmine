package badgerdb

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	"github.com/ArkLabsHQ/ark-node/pkg/vhtlc"
	"github.com/ark-network/ark/common"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

type badgerRepo struct {
	store *badgerhold.Store
}

func NewVHTLCRepo(baseDir string, logger badger.Logger) (domain.VHTLCRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, "vhtlc")
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open vHTLC store: %s", err)
	}
	return &badgerRepo{store}, nil
}

// GetAll retrieves all VHTLC options from the database
func (r *badgerRepo) GetAll(ctx context.Context) ([]vhtlc.Opts, error) {
	var opts []data
	err := r.store.Find(&opts, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all vHTLC options: %w", err)
	}

	var vOpts []vhtlc.Opts
	for _, opt := range opts {
		vOpt, err := opt.toOpts()
		if err != nil {
			return nil, fmt.Errorf("failed to convert data to opts: %w", err)
		}
		vOpts = append(vOpts, *vOpt)
	}
	return vOpts, nil
}

// Get retrieves a specific VHTLC option by preimage hash
func (r *badgerRepo) Get(ctx context.Context, preimageHash string) (*vhtlc.Opts, error) {
	var dataOpts data
	err := r.store.Get(preimageHash, &dataOpts)
	if err == badgerhold.ErrNotFound {
		return nil, fmt.Errorf("vHTLC with preimage hash %s not found", preimageHash)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get vHTLC option: %w", err)
	}

	opts, err := dataOpts.toOpts()
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to opts: %w", err)
	}

	return opts, nil
}

// Add stores a new VHTLC option in the database
func (r *badgerRepo) Add(ctx context.Context, opts vhtlc.Opts) error {
	data := data{
		PreimageHash:                         hex.EncodeToString(opts.PreimageHash),
		Sender:                               hex.EncodeToString(opts.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(opts.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(opts.Server.SerializeCompressed()),
		RefundLocktime:                       opts.RefundLocktime,
		UnilateralClaimDelay:                 opts.UnilateralClaimDelay,
		UnilateralRefundDelay:                opts.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: opts.UnilateralRefundWithoutReceiverDelay,
	}

	return r.store.Insert(data.PreimageHash, data)
}

// Delete removes a VHTLC option from the database
func (r *badgerRepo) Delete(ctx context.Context, preimageHash string) error {
	return r.store.Delete(preimageHash, data{})
}

type data struct {
	PreimageHash                         string
	Sender                               string
	Receiver                             string
	Server                               string
	RefundLocktime                       common.AbsoluteLocktime
	UnilateralClaimDelay                 common.RelativeLocktime
	UnilateralRefundDelay                common.RelativeLocktime
	UnilateralRefundWithoutReceiverDelay common.RelativeLocktime
}

func (d *data) toOpts() (*vhtlc.Opts, error) {
	senderBytes, err := hex.DecodeString(d.Sender)
	if err != nil {
		return nil, err
	}
	receiverBytes, err := hex.DecodeString(d.Receiver)
	if err != nil {
		return nil, err
	}
	serverBytes, err := hex.DecodeString(d.Server)
	if err != nil {
		return nil, err
	}

	sender, err := secp256k1.ParsePubKey(senderBytes)
	if err != nil {
		return nil, err
	}
	receiver, err := secp256k1.ParsePubKey(receiverBytes)
	if err != nil {
		return nil, err
	}
	server, err := secp256k1.ParsePubKey(serverBytes)
	if err != nil {
		return nil, err
	}

	preimageHashBytes, err := hex.DecodeString(d.PreimageHash)
	if err != nil {
		return nil, err
	}

	return &vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               server,
		RefundLocktime:                       d.RefundLocktime,
		UnilateralClaimDelay:                 d.UnilateralClaimDelay,
		UnilateralRefundDelay:                d.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: d.UnilateralRefundWithoutReceiverDelay,
		PreimageHash:                         preimageHashBytes,
	}, nil
}
