// Levier — internal/workflow
//
// RunTaskQueue sentinel and classification errors.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNoTask means no ready task is currently available.
	ErrNoTask = errors.New("workflow: no task available")
	// ErrLeaseLost means the caller no longer owns the task lease.
	ErrLeaseLost = errors.New("workflow: lease lost")
	// ErrTaskNotFound means the task does not exist.
	ErrTaskNotFound = errors.New("workflow: task not found")
	// ErrTaskTerminal means the task is already DONE, DEAD, or CANCELLED.
	ErrTaskTerminal = errors.New("workflow: task is terminal")
	// ErrDuplicateActiveTask means an active task already exists for the key.
	ErrDuplicateActiveTask = errors.New("workflow: duplicate active task")
	// ErrInvalidTask means the task failed local validation.
	ErrInvalidTask = errors.New("workflow: invalid task")
)

// RetryAfter wraps err with an explicit retry delay.
func RetryAfter(err error, delay time.Duration) error {
	if err == nil {
		return nil
	}
	return &retryAfterError{err: err, delay: delay}
}

// RetryAfterDuration extracts an explicit retry delay.
func RetryAfterDuration(err error) (time.Duration, bool) {
	var target *retryAfterError
	if errors.As(err, &target) {
		return target.delay, true
	}
	return 0, false
}

// Permanent marks err as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err is marked non-retryable.
func IsPermanent(err error) bool {
	var target *permanentError
	return errors.As(err, &target)
}

type retryAfterError struct {
	err   error
	delay time.Duration
}

func (e *retryAfterError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", e.err, e.delay)
}

func (e *retryAfterError) Unwrap() error {
	return e.err
}

type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return fmt.Sprintf("%v (permanent)", e.err)
}

func (e *permanentError) Unwrap() error {
	return e.err
}
