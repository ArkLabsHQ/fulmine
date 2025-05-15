package badgerdb

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

const (
	swapDir = "swap"
)

type swapRepository struct {
	store *badgerhold.Store
}

func NewSwapRepository(baseDir string, logger badger.Logger) (domain.SwapRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, swapDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open swap store: %s", err)
	}
	return &swapRepository{store}, nil
}

// GetAll retrieves all VHTLC options from the database
func (r *swapRepository) GetAll(ctx context.Context) ([]domain.Swap, error) {
	var swaps []domain.Swap
	err := r.store.Find(&swaps, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Swaps: %w", err)
	}

	// TODO: Verify if this is needed
	// var vOpts []vhtlc.Opts
	// for _, opt := range opts {
	// 	vOpt, err := opt.toOpts()
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to convert data to opts: %w", err)
	// 	}
	// 	vOpts = append(vOpts, *vOpt)
	// }
	return swaps, nil
}

// Get retrieves a specific VHTLC option by preimage hash
func (r *swapRepository) Get(ctx context.Context, invoice string) (*domain.Swap, error) {
	var swap domain.Swap
	err := r.store.Get(invoice, &swap)
	if err == badgerhold.ErrNotFound {
		return nil, fmt.Errorf("swap with invoice %s not found", invoice)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Swap: %w", err)
	}

	// TODO: Verify if this is needed
	// opts, err := dataOpts.toOpts()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to convert data to opts: %w", err)
	// }

	return &swap, nil
}

// Add stores a new Swap in the database
func (r *swapRepository) Add(ctx context.Context, swap domain.Swap) error {

	return r.store.Insert(swap.Invoice, swap)
}

// Delete removes a Swap from the database
func (r *swapRepository) Delete(ctx context.Context, invoice string) error {
	return r.store.Delete(invoice, domain.Swap{})
}

func (s *swapRepository) Close() {
	// nolint:all
	s.store.Close()
}

// type data struct {
// 	PreimageHash                         string
// 	Sender                               string
// 	Receiver                             string
// 	Server                               string
// 	RefundLocktime                       common.AbsoluteLocktime
// 	UnilateralClaimDelay                 common.RelativeLocktime
// 	UnilateralRefundDelay                common.RelativeLocktime
// 	UnilateralRefundWithoutReceiverDelay common.RelativeLocktime
// }

// func (d *data) toOpts() (*vhtlc.Opts, error) {
// 	senderBytes, err := hex.DecodeString(d.Sender)
// 	if err != nil {
// 		return nil, err
// 	}
// 	receiverBytes, err := hex.DecodeString(d.Receiver)
// 	if err != nil {
// 		return nil, err
// 	}
// 	serverBytes, err := hex.DecodeString(d.Server)
// 	if err != nil {
// 		return nil, err
// 	}

// 	sender, err := secp256k1.ParsePubKey(senderBytes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	receiver, err := secp256k1.ParsePubKey(receiverBytes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	server, err := secp256k1.ParsePubKey(serverBytes)
// 	if err != nil {
// 		return nil, err
// 	}

// 	preimageHashBytes, err := hex.DecodeString(d.PreimageHash)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &vhtlc.Opts{
// 		Sender:                               sender,
// 		Receiver:                             receiver,
// 		Server:                               server,
// 		RefundLocktime:                       d.RefundLocktime,
// 		UnilateralClaimDelay:                 d.UnilateralClaimDelay,
// 		UnilateralRefundDelay:                d.UnilateralRefundDelay,
// 		UnilateralRefundWithoutReceiverDelay: d.UnilateralRefundWithoutReceiverDelay,
// 		PreimageHash:                         preimageHashBytes,
// 	}, nil
// }
