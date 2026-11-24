// Levier — internal/runstore
//
// RunStore error values.
//
// Copyright 2026 The Levier Authors.
package runstore

import "errors"

var (
	// ErrRunNotFound reports a missing run.
	ErrRunNotFound = errors.New("run not found")
	// ErrAppNotFound reports a missing app.
	ErrAppNotFound = errors.New("app not found")
	// ErrSessionNotFound reports a missing session.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionConflict reports a session owned by another app or tenant.
	ErrSessionConflict = errors.New("session belongs to another app or tenant")
	// ErrSessionClosed reports a session that was explicitly closed.
	ErrSessionClosed = errors.New("session is closed")
	// ErrSessionActive reports a session that still has active runs.
	ErrSessionActive = errors.New("session has active runs")
	// ErrCheckpointNotFound reports a session without a stored checkpoint.
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	// ErrCheckpointConflict reports a session owned by another app or tenant.
	ErrCheckpointConflict = errors.New("checkpoint session belongs to another app or tenant")
	// ErrIdempotencyConflict reports the same key with a different request.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrInvalidRun reports invalid run input.
	ErrInvalidRun = errors.New("invalid run")
	// ErrRunNotAssignable reports a run in a non-dispatchable state.
	ErrRunNotAssignable = errors.New("run is not assignable")
	// ErrRunNotResumable reports a run that is not waiting.
	ErrRunNotResumable = errors.New("run is not waiting")
	// ErrInvalidEvent reports invalid event input.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrEphemeralEvent reports an attempt to persist an ephemeral event.
	ErrEphemeralEvent = errors.New("ephemeral events are not persisted")
)
