// Levier - Runtime placement and connection boundaries.
//
// Copyright 2026 The Levier Authors.
package ports

import runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"

// RuntimePlacement selects and reserves a Runtime connection.
type RuntimePlacement interface {
	PickHealthy(runtimeType runtimev1.RuntimeType, runtimeName string, labels map[string]string) (string, bool)
	Reserve(daemonID string) bool
	Disconnect(daemonID string)
}
