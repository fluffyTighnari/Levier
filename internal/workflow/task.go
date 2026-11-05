// Middleware — internal/workflow
//
// RunTaskQueue task model and validation.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"fmt"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
)

// TaskType identifies one Run-level task.
type TaskType string

const (
	// TaskDispatch selects a daemon and sends TaskAssign.
	TaskDispatch TaskType = "RUN_DISPATCH"
	// TaskRetry requeues a run after a daemon loss or rejection.
	TaskRetry TaskType = "RUN_RETRY"
	// TaskTimeout enforces the run deadline.
	TaskTimeout TaskType = "RUN_TIMEOUT"
	// TaskReconcile reconciles a reconnected daemon.
	TaskReconcile TaskType = "RUN_RECONCILE"
)

// Valid reports whether t is a supported task type.
func (t TaskType) Valid() bool {
	switch t {
	case TaskDispatch, TaskRetry, TaskTimeout, TaskReconcile:
		return true
	default:
		return false
	}
}

// DefaultMaxAttempts returns the v1 default delivery budget.
func (t TaskType) DefaultMaxAttempts() int {
	switch t {
	case TaskDispatch:
		return 5
	case TaskRetry, TaskTimeout, TaskReconcile:
		return 3
	default:
		return 1
	}
}

// Status is the lifecycle state of a task.
type Status string

const (
	// StatusPending is ready or waiting for available_at.
	StatusPending Status = "PENDING"
	// StatusLeased is owned by a worker until lease_expires_at.
	StatusLeased Status = "LEASED"
	// StatusDone completed successfully.
	StatusDone Status = "DONE"
	// StatusDead exhausted retries or hit a permanent error.
	StatusDead Status = "DEAD"
	// StatusCancelled was explicitly cancelled.
	StatusCancelled Status = "CANCELLED"
)

// Valid reports whether s is a supported status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusLeased, StatusDone, StatusDead, StatusCancelled:
		return true
	default:
		return false
	}
}

// Active reports whether s is PENDING or LEASED.
func (s Status) Active() bool {
	return s == StatusPending || s == StatusLeased
}

// Terminal reports whether s is DONE, DEAD, or CANCELLED.
func (s Status) Terminal() bool {
	return s == StatusDone || s == StatusDead || s == StatusCancelled
}

// Task is one durable RunTaskQueue row.
type Task struct {
	ID             string
	RunID          string
	Type           TaskType
	Status         Status
	AvailableAt    time.Time
	Attempt        int
	MaxAttempts    int
	LeaseOwner     string
	LeaseToken     uint64
	LeaseExpiresAt time.Time
	LastError      string
	Payload        map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewTask builds a pending task with a generated ULID.
func NewTask(runID string, taskType TaskType, availableAt time.Time, maxAttempts int) Task {
	now := time.Now().UTC()
	if availableAt.IsZero() {
		availableAt = now
	}
	if maxAttempts <= 0 {
		maxAttempts = taskType.DefaultMaxAttempts()
	}
	return Task{
		ID:          id.New(),
		RunID:       runID,
		Type:        taskType,
		Status:      StatusPending,
		AvailableAt: availableAt.UTC(),
		MaxAttempts: maxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// IsActive reports whether the task is PENDING or LEASED.
func (t Task) IsActive() bool {
	return t.Status.Active()
}

// Validate checks task invariants before persistence or worker handling.
func (t Task) Validate() error {
	if _, err := id.Parse(t.ID); err != nil {
		return fmt.Errorf("task.id: %w", err)
	}
	if t.RunID == "" {
		return fmt.Errorf("task.run_id is required")
	}
	if !t.Type.Valid() {
		return fmt.Errorf("task.type %q is invalid", t.Type)
	}
	if !t.Status.Valid() {
		return fmt.Errorf("task.status %q is invalid", t.Status)
	}
	if t.MaxAttempts <= 0 {
		return fmt.Errorf("task.max_attempts must be positive")
	}
	if t.Attempt < 0 {
		return fmt.Errorf("task.attempt must not be negative")
	}
	if t.Attempt > t.MaxAttempts {
		return fmt.Errorf("task.attempt %d exceeds max_attempts %d", t.Attempt, t.MaxAttempts)
	}
	if t.Status == StatusLeased {
		if t.LeaseOwner == "" {
			return fmt.Errorf("task.lease_owner is required when LEASED")
		}
		if t.LeaseToken == 0 {
			return fmt.Errorf("task.lease_token is required when LEASED")
		}
		if t.LeaseExpiresAt.IsZero() {
			return fmt.Errorf("task.lease_expires_at is required when LEASED")
		}
	}
	return nil
}
