package badgerdb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

type vhtlcRepository struct {
	store *badgerhold.Store
}

func NewVHTLCRepository(baseDir string, logger badger.Logger) (domain.VHTLCRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, "vhtlc")
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open vHTLC store: %s", err)
	}
	return &vhtlcRepository{store}, nil
}

// GetAll retrieves all VHTLC options from the database
func (r *vhtlcRepository) GetAll(ctx context.Context) ([]vhtlc.Opts, error) {
	var opts []vhtlcData
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
func (r *vhtlcRepository) Get(ctx context.Context, preimageHash string) (*vhtlc.Opts, error) {
	var dataOpts vhtlcData
	err := r.store.Get(preimageHash, &dataOpts)
	if errors.Is(err, badgerhold.ErrNotFound) {
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
func (r *vhtlcRepository) Add(ctx context.Context, opts vhtlc.Opts) error {
	data := vhtlcData{
		PreimageHash:                         hex.EncodeToString(opts.PreimageHash),
		Sender:                               hex.EncodeToString(opts.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(opts.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(opts.Server.SerializeCompressed()),
		RefundLocktime:                       opts.RefundLocktime,
		UnilateralClaimDelay:                 opts.UnilateralClaimDelay,
		UnilateralRefundDelay:                opts.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: opts.UnilateralRefundWithoutReceiverDelay,
	}

	if err := r.store.Insert(data.PreimageHash, data); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("vHTLC with preimage hash %s already exists", data.PreimageHash)
		}
		return err
	}
	return nil
}

// Delete removes a VHTLC option from the database
func (r *vhtlcRepository) Delete(ctx context.Context, preimageHash string) error {
	return r.store.Delete(preimageHash, vhtlcData{})
}

func (s *vhtlcRepository) Close() {
	// nolint:all
	s.store.Close()
}

type vhtlcData struct {
	PreimageHash                         string
	Sender                               string
	Receiver                             string
	Server                               string
	RefundLocktime                       arklib.AbsoluteLocktime
	UnilateralClaimDelay                 arklib.RelativeLocktime
	UnilateralRefundDelay                arklib.RelativeLocktime
	UnilateralRefundWithoutReceiverDelay arklib.RelativeLocktime
}

func (d *vhtlcData) toOpts() (*vhtlc.Opts, error) {
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

	sender, err := btcec.ParsePubKey(senderBytes)
	if err != nil {
		return nil, err
	}
	receiver, err := btcec.ParsePubKey(receiverBytes)
	if err != nil {
		return nil, err
	}
	server, err := btcec.ParsePubKey(serverBytes)
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
