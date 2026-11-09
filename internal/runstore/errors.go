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
	// ErrSessionNotFound reports a missing session.
	ErrSessionNotFound = errors.New("session not found")
	// ErrIdempotencyConflict reports the same key with a different request.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrInvalidRun reports invalid run input.
	ErrInvalidRun = errors.New("invalid run")
	// ErrRunNotAssignable reports a run in a non-dispatchable state.
	ErrRunNotAssignable = errors.New("run is not assignable")
	// ErrInvalidEvent reports invalid event input.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrEphemeralEvent reports an attempt to persist an ephemeral event.
	ErrEphemeralEvent = errors.New("ephemeral events are not persisted")
)
