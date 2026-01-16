package domain

import (
	"context"
	"time"

	"github.com/btcsuite/btcd/wire"
)

type Intent struct {
	Message string
	Proof   string
}

type DelegateTaskStatus string
const (
	DelegateTaskStatusPending DelegateTaskStatus = "pending"
	DelegateTaskStatusDone DelegateTaskStatus = "done"
	DelegateTaskStatusFailed DelegateTaskStatus = "failed"
	DelegateTaskStatusCancelled DelegateTaskStatus = "cancelled"
)

type DelegateTask struct {
	ID string
	
	Intent Intent

	Inputs []wire.OutPoint
	ForfeitTxs map[wire.OutPoint]string // forfeit transaction per input
	Fee uint64
	DelegatorPublicKey string
	ScheduledAt time.Time

	Status DelegateTaskStatus
	FailReason string
}

type PendingDelegateTask struct {
	ID string
	ScheduledAt time.Time
}

type DelegatorRepository interface {
	Add(ctx context.Context, task DelegateTask) error
	GetByID(ctx context.Context, id string) (*DelegateTask, error)
	// return status == pending tasks
	GetAllPending(ctx context.Context) ([]PendingDelegateTask, error)
	// return pending tasks that have any of the given inputs
	GetPendingTaskIDsByInputs(ctx context.Context, inputs []wire.OutPoint) ([]string, error)
	CancelTasks(ctx context.Context, ids... string) error
	SuccessTasks(ctx context.Context, ids... string) error
	FailTasks(ctx context.Context, reason string, ids... string) error
	Close()
}
