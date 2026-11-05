// Middleware — internal/workflow
//
// RunTaskQueue persistence contract.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"time"
)

// Stats summarizes queue depth and oldest age.
type Stats struct {
	Total            int64              `json:"total"`
	ByStatus         map[Status]int64   `json:"by_status"`
	ByType           map[TaskType]int64 `json:"by_type"`
	OldestPending    time.Time          `json:"oldest_pending"`
	OldestPendingAge time.Duration      `json:"oldest_pending_age"`
}

// Store is the durable RunTaskQueue contract.
type Store interface {
	// Enqueue creates a task or returns the existing active task for
	// the same (run_id, task_type).
	Enqueue(ctx context.Context, task Task) (Task, error)

	// Claim leases one ready task to owner for ttl.
	Claim(ctx context.Context, owner string, ttl time.Duration) (Task, Lease, error)

	// Extend renews a lease owned by owner/token.
	Extend(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error)

	// Ack marks a task DONE.
	Ack(ctx context.Context, task Task, lease Lease) error

	// Nack returns a task to PENDING or DEAD when attempts are exhausted.
	Nack(ctx context.Context, task Task, lease Lease, retryAt time.Time, cause error) error

	// Fail marks a task DEAD without retry.
	Fail(ctx context.Context, task Task, lease Lease, cause error) error

	// CancelRun cancels all active tasks for a run.
	CancelRun(ctx context.Context, runID, reason string) (int, error)

	// CancelTask cancels one active task type for a run.
	CancelTask(ctx context.Context, runID string, taskType TaskType, reason string) (int, error)

	// ReapExpired returns expired leases to PENDING or DEAD.
	ReapExpired(ctx context.Context, now time.Time, limit int) (int, error)

	// Stats returns queue depth and age metrics.
	Stats(ctx context.Context) (Stats, error)
}

// Lease is the worker's ownership proof for one task.
type Lease struct {
	TaskID    string
	Owner     string
	Token     uint64
	ExpiresAt time.Time
}

// DeadLetterHandler receives tasks that reached DEAD.
type DeadLetterHandler interface {
	HandleDead(ctx context.Context, task Task, cause error) error
}

// DeadLetterFunc adapts a function to DeadLetterHandler.
type DeadLetterFunc func(context.Context, Task, error) error

// HandleDead implements DeadLetterHandler.
func (f DeadLetterFunc) HandleDead(ctx context.Context, task Task, cause error) error {
	return f(ctx, task, cause)
}
