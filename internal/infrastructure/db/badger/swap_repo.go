package badgerdb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
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

func (r *swapRepository) GetAll(ctx context.Context) ([]domain.Swap, error) {
	var swapDataList []swapData
	err := r.store.Find(&swapDataList, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all swaps: %w", err)
	}

	var swaps []domain.Swap
	for _, s := range swapDataList {
		swap, err := s.toSwap()
		if err != nil {
			return nil, fmt.Errorf("failed to convert data to swap: %w", err)
		}

		swaps = append(swaps, *swap)
	}
	return swaps, nil
}

func (r *swapRepository) Get(ctx context.Context, swapId string) (*domain.Swap, error) {
	var swapData swapData
	err := r.store.Get(swapId, &swapData)
	if errors.Is(err, badgerhold.ErrNotFound) {
		return nil, fmt.Errorf("swap %s not found", swapId)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get swap: %w", err)
	}

	swap, err := swapData.toSwap()
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to swap: %w", err)
	}

	return swap, nil
}

// Add stores a new Swap in the database
func (r *swapRepository) Add(ctx context.Context, swap domain.Swap) error {
	swapData := toSwapData(swap)

	if err := r.store.Insert(swap.Id, swapData); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("swap %s already exists", swap.Id)
		}
		return err
	}
	return nil
}

func (s *swapRepository) Close() {
	// nolint:all
	s.store.Close()
}

type swapData struct {
	Id          string
	Amount      uint64
	Timestamp   int64
	To          boltz.Currency
	From        boltz.Currency
	Status      domain.SwapStatus
	Invoice     string
	VhtlcOpts   vhtlcData
	FundingTxId string
	RedeemTxId  string
}

func toSwapData(swap domain.Swap) swapData {
	vhtlcOpts := swap.VhtlcOpts

	vhtlcData := vhtlcData{
		PreimageHash:                         hex.EncodeToString(vhtlcOpts.PreimageHash),
		Sender:                               hex.EncodeToString(vhtlcOpts.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(vhtlcOpts.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(vhtlcOpts.Server.SerializeCompressed()),
		RefundLocktime:                       vhtlcOpts.RefundLocktime,
		UnilateralClaimDelay:                 vhtlcOpts.UnilateralClaimDelay,
		UnilateralRefundDelay:                vhtlcOpts.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: vhtlcOpts.UnilateralRefundWithoutReceiverDelay,
	}

	return swapData{
		Id:          swap.Id,
		Amount:      swap.Amount,
		Timestamp:   swap.Timestamp,
		To:          swap.To,
		From:        swap.From,
		Status:      swap.Status,
		Invoice:     swap.Invoice,
		VhtlcOpts:   vhtlcData,
		FundingTxId: swap.FundingTxId,
		RedeemTxId:  swap.RedeemTxId,
	}
}

func (s *swapData) toSwap() (*domain.Swap, error) {
	vhtlcOps, err := s.VhtlcOpts.toOpts()
	if err != nil {
		return nil, err
	}

	return &domain.Swap{
		Id:          s.Id,
		Amount:      s.Amount,
		Timestamp:   s.Timestamp,
		To:          s.To,
		From:        s.From,
		Status:      s.Status,
		Invoice:     s.Invoice,
		VhtlcOpts:   *vhtlcOps,
		FundingTxId: s.FundingTxId,
		RedeemTxId:  s.RedeemTxId,
	}, nil
}
