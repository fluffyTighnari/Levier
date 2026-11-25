// Levier - cluster registry errors.
// Copyright 2026 The Levier Authors.
package cluster

import "errors"

var (
	// ErrLeaseLost reports a stale gateway / runtime / app connection epoch.
	ErrLeaseLost = errors.New("cluster lease is no longer owned")
	// ErrLeaseHeld reports a lease owned by another active owner.
	ErrLeaseHeld = errors.New("cluster lease is held by another owner")
	// ErrCapacityExceeded reports a Runtime connection with no free slot.
	ErrCapacityExceeded = errors.New("runtime capacity is exhausted")
	// ErrInvalidRegistration reports missing or invalid registry input.
	ErrInvalidRegistration = errors.New("invalid cluster registration")
	// ErrConnectionNotFound reports a missing connection row.
	ErrConnectionNotFound = errors.New("cluster connection not found")
)
