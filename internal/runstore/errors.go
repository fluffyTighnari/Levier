// Middleware — internal/runstore
//
// RunStore error values.
//
// Copyright 2026 The Middleware Authors.
package runstore

import "errors"

var (
	// ErrRunNotFound reports a missing run.
	ErrRunNotFound = errors.New("run not found")
	// ErrAppNotFound reports a missing app.
	ErrAppNotFound = errors.New("app not found")
	// ErrIdempotencyConflict reports the same key with a different request.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrInvalidRun reports invalid run input.
	ErrInvalidRun = errors.New("invalid run")
	// ErrInvalidEvent reports invalid event input.
	ErrInvalidEvent = errors.New("invalid event")
)
