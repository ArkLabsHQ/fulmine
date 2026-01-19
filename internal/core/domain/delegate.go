package domain

import (
	"context"
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
	// return pending tasks that have any of the given inputs
	GetPendingTaskIDsByInputs(ctx context.Context, inputs []wire.OutPoint) ([]string, error)
	CancelTasks(ctx context.Context, ids... string) error
	CompleteTasks(ctx context.Context, commitmentTxid string, ids... string) error
	FailTasks(ctx context.Context, reason string, ids... string) error
	Close()
}
