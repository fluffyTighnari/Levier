// Levier - cluster registry errors.
// Copyright 2026 The Levier Authors.
package cluster

import "errors"

var (
	// ErrLeaseLost reports a stale gateway / runtime / app connection epoch.
	ErrLeaseLost = errors.New("cluster lease is no longer owned")
	// ErrLeaseHeld reports an App execution lease owned by another connection.
	ErrLeaseHeld = errors.New("app execution lease is held by another connection")
	// ErrInvalidRegistration reports missing or invalid registry input.
	ErrInvalidRegistration = errors.New("invalid cluster registration")
	// ErrConnectionNotFound reports a missing connection row.
	ErrConnectionNotFound = errors.New("cluster connection not found")
)
