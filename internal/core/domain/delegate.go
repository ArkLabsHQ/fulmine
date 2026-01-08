package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/wire"
)

var (
	ErrDelegateTaskNotPending = fmt.Errorf("delegate task is not pending")
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
	ForfeitTx string

	Inputs []wire.OutPoint
	Fee uint64
	DelegatorPublicKey string
	ScheduledAt time.Time

	Status DelegateTaskStatus
	FailReason string
}

func (d *DelegateTask) Fail(reason string) error {
	if d.Status != DelegateTaskStatusPending {
		return ErrDelegateTaskNotPending
	}

	d.Status = DelegateTaskStatusFailed
	d.FailReason = reason
	return nil
}

func (d *DelegateTask) Success() error {
	if d.Status != DelegateTaskStatusPending {
		return ErrDelegateTaskNotPending
	}

	d.Status = DelegateTaskStatusDone
	return nil
}

func (d *DelegateTask) Cancel() error {
	if d.Status != DelegateTaskStatusPending {
		return ErrDelegateTaskNotPending
	}

	d.Status = DelegateTaskStatusCancelled
	return nil
}

type PendingDelegateTask struct {
	ID string
	ScheduledAt time.Time
}

type DelegatorRepository interface {
	AddOrUpdate(ctx context.Context, task DelegateTask) error
	GetByID(ctx context.Context, id string) (*DelegateTask, error)
	// return status == pending tasks
	GetAllPending(ctx context.Context) ([]PendingDelegateTask, error) 
	Close()
}
