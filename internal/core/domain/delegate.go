package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/wire"
)

type Intent struct {
	Message string
	Proof   string
	Txid string
	Inputs []wire.OutPoint
}

type DelegateTaskStatus int
const (
	DelegateTaskStatusPending DelegateTaskStatus = iota
	DelegateTaskStatusCompleted
	DelegateTaskStatusFailed
	DelegateTaskStatusCancelled
)

// String returns the string representation of the status
func (s DelegateTaskStatus) String() string {
	switch s {
	case DelegateTaskStatusPending:
		return "pending"
	case DelegateTaskStatusCompleted:
		return "completed"
	case DelegateTaskStatusFailed:
		return "failed"
	case DelegateTaskStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// DelegateTaskStatusFromString parses string to DelegateTaskStatus
func DelegateTaskStatusFromString(s string) (DelegateTaskStatus, error) {
	statusStr := strings.ToLower(strings.TrimSpace(s))
	
	switch statusStr {
	case "pending":
		return DelegateTaskStatusPending, nil
	case "completed":
		return DelegateTaskStatusCompleted, nil
	case "failed":
		return DelegateTaskStatusFailed, nil
	case "cancelled", "canceled":
		return DelegateTaskStatusCancelled, nil
	default:
		return DelegateTaskStatusPending, fmt.Errorf("invalid status: %s. Must be one of: pending, completed, failed, cancelled", s)
	}
}

type DelegateTask struct {
	ID string
	Intent Intent
	ForfeitTxs map[wire.OutPoint]string // forfeit transaction per input
	Fee uint64
	DelegatorPublicKey string
	ScheduledAt time.Time
	Status DelegateTaskStatus
	FailReason string // set only when task is failed
	CommitmentTxid string // set only when task is completed
}

type PendingDelegateTask struct {
	ID string
	ScheduledAt time.Time
}

type DelegateRepository interface {
	Add(ctx context.Context, task DelegateTask) error
	GetByID(ctx context.Context, id string) (*DelegateTask, error)
	// return status == pending tasks
	GetAllPending(ctx context.Context) ([]PendingDelegateTask, error)
	GetAll(ctx context.Context, status DelegateTaskStatus, limit int, offset int) ([]DelegateTask, error)
	GetPendingTaskByIntentTxID(ctx context.Context, txid string) (*PendingDelegateTask, error)
	// return pending tasks that have any of the given inputs
	GetPendingTaskIDsByInputs(ctx context.Context, inputs []wire.OutPoint) ([]string, error)
	CancelTasks(ctx context.Context, ids... string) error
	CompleteTasks(ctx context.Context, commitmentTxid string, ids... string) error
	FailTasks(ctx context.Context, reason string, ids... string) error
	Close()
}
