package domain

import (
	"context"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
)

type ChainSwapStatus int

const (
	// Pending states
	ChainSwapPending ChainSwapStatus = iota
	ChainSwapUserLocked
	ChainSwapServerLocked

	// Success states
	ChainSwapClaimed

	// Failed states
	ChainSwapUserLockedFailed
	ChainSwapFailed
	ChainSwapRefundFailed
	ChainSwapRefunded
	ChainSwapRefundedUnilaterally
)

type ChainSwap struct {
	Id string

	From          boltz.Currency
	To            boltz.Currency
	ClaimPreimage string

	Amount uint64

	UserBtcLockupAddress string

	UserLockupTxId   string
	ServerLockupTxId string
	ClaimTxId        string
	RefundTxId       string

	CreatedAt    int64
	Status       ChainSwapStatus
	ErrorMessage string

	BoltzCreateResponseJSON string
}

// IsComplete returns true if swap is in a terminal state
func (cs *ChainSwap) IsComplete() bool {
	return cs.Status == ChainSwapClaimed ||
		cs.Status == ChainSwapRefunded ||
		cs.Status == ChainSwapFailed
}

// CanRefund returns true if swap can be refunded
func (cs *ChainSwap) CanRefund() bool {
	return cs.Status == ChainSwapPending ||
		cs.Status == ChainSwapUserLocked ||
		cs.Status == ChainSwapServerLocked
}

// IsPending returns true if swap is still in progress
func (cs *ChainSwap) IsPending() bool {
	return !cs.IsComplete()
}

// UserLocked updates the swap when user locks funds
func (cs *ChainSwap) UserLocked(txid string) {
	cs.UserLockupTxId = txid
	cs.Status = ChainSwapUserLocked
}

// ServerLocked updates the swap when server locks funds
func (cs *ChainSwap) ServerLocked(txid string) {
	cs.ServerLockupTxId = txid
	cs.Status = ChainSwapServerLocked
}

// Claimed updates the swap when funds are successfully claimed
func (cs *ChainSwap) Claimed(txid string) {
	cs.ClaimTxId = txid
	cs.Status = ChainSwapClaimed
}

// Refunded updates the swap when funds are refunded
func (cs *ChainSwap) Refunded(txid string) {
	cs.RefundTxId = txid
	cs.Status = ChainSwapRefunded
}

// RefundedUnilaterally updates the swap when funds are refunded
func (cs *ChainSwap) RefundedUnilaterally(txid string) {
	cs.RefundTxId = txid
	cs.Status = ChainSwapRefundedUnilaterally
}

// Failed marks the swap as failed with an error message
func (cs *ChainSwap) Failed(errorMsg string) {
	cs.Status = ChainSwapFailed
	cs.ErrorMessage = errorMsg
}

// RefundFailed marks the swap as refund failed
func (cs *ChainSwap) RefundFailed(errorMsg string) {
	cs.Status = ChainSwapRefundFailed
	cs.ErrorMessage = errorMsg
}

// UserLockFailed marks the swap as user lock failed
func (cs *ChainSwap) UserLockFailed(errorMsg string) {
	cs.Status = ChainSwapUserLockedFailed
	cs.ErrorMessage = errorMsg
}

// ChainSwapRepository stores chain swaps initiated by the wallet
type ChainSwapRepository interface {
	// Add creates a new chain swap record
	Add(ctx context.Context, swap ChainSwap) error

	// Get retrieves a chain swap by ID
	Get(ctx context.Context, id string) (*ChainSwap, error)

	// GetAll retrieves all chain swaps
	GetAll(ctx context.Context) ([]ChainSwap, error)

	// GetByIDs retrieves chain swaps filtered by IDs
	GetByIDs(ctx context.Context, ids []string) ([]ChainSwap, error)

	// GetByStatus retrieves chain swaps filtered by status
	GetByStatus(ctx context.Context, status ChainSwapStatus) ([]ChainSwap, error)

	// Update updates an existing chain swap
	Update(ctx context.Context, swap ChainSwap) error

	// Delete removes a chain swap (rarely used, kept for completeness)
	Delete(ctx context.Context, id string) error

	// Close closes the repository
	Close()
}
