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

func (r *swapRepository) Add(ctx context.Context, swaps []domain.Swap) (int, error) {
	if len(swaps) == 0 {
		return -1, nil
	}

	count := 0
	for _, swap := range swaps {
		swapData := toSwapData(swap)
		if err := r.store.Insert(swap.Id, swapData); err != nil {
			if errors.Is(err, badgerhold.ErrKeyExists) {
				continue
			}
			return -1, fmt.Errorf("failed to insert swap %s: %w", swap.Id, err)
		}
		count++
	}

	return count, nil
}

func (r *swapRepository) Update(ctx context.Context, swap domain.Swap) error {
	swapData := toSwapData(swap)
	return r.store.Update(swap.Id, swapData)
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
	Type        domain.SwapType
	Invoice     string
	Vhtlc       vhtlcData
	FundingTxId string
	RedeemTxId  string
}

func toSwapData(swap domain.Swap) swapData {
	vHTLC := swap.Vhtlc

	vhtlcData := vhtlcData{
		Id:                                   vHTLC.Id,
		PreimageHash:                         hex.EncodeToString(vHTLC.PreimageHash),
		Sender:                               hex.EncodeToString(vHTLC.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(vHTLC.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(vHTLC.Server.SerializeCompressed()),
		RefundLocktime:                       vHTLC.RefundLocktime,
		UnilateralClaimDelay:                 vHTLC.UnilateralClaimDelay,
		UnilateralRefundDelay:                vHTLC.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: vHTLC.UnilateralRefundWithoutReceiverDelay,
	}

	return swapData{
		Id:          swap.Id,
		Amount:      swap.Amount,
		Timestamp:   swap.Timestamp,
		To:          swap.To,
		From:        swap.From,
		Status:      swap.Status,
		Type:        swap.Type,
		Invoice:     swap.Invoice,
		Vhtlc:       vhtlcData,
		FundingTxId: swap.FundingTxId,
		RedeemTxId:  swap.RedeemTxId,
	}
}

func (s *swapData) toSwap() (*domain.Swap, error) {
	vHTLC, err := s.Vhtlc.toVhtlc()
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
		Type:        s.Type,
		Invoice:     s.Invoice,
		Vhtlc:       vHTLC,
		FundingTxId: s.FundingTxId,
		RedeemTxId:  s.RedeemTxId,
	}, nil
}
