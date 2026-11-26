// Package ports defines the small boundaries between Levier roles.
//
// The all-in-one process can bind these ports to local adapters; split
// deployments can bind the same ports to internal RPC clients.
//
// Copyright 2026 The Levier Authors.
package ports

import (
	"context"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

// RuntimeTransport sends control envelopes to one Runtime connection.
type RuntimeTransport interface {
	Send(daemonID string, env *runtimev1.Envelope) error
	Release(daemonID, reservationID string)
}

// RuntimeDispatcher is the synchronous TaskAssign path used when a Runtime
// Gateway owns the daemon stream. Control replicas do not need a process-local
// pending-ack map when the transport implements this interface.
type RuntimeDispatcher interface {
	Dispatch(ctx context.Context, daemonID string, env *runtimev1.Envelope, timeout time.Duration) (*runtimev1.TaskAck, error)
}
