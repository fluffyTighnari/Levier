// Levier - local adapters for the split Runtime ports.
//
// The in-process hub keeps its historical Reserve/Release methods. These
// adapters translate them to the narrower ports used by a split gateway.
//
// Copyright 2026 The Levier Authors.
package control

import (
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/runtime/ports"
)

type hubTransport struct {
	hub DaemonHub
}

func (t hubTransport) Send(daemonID string, env *runtimev1.Envelope) error {
	return t.hub.Send(daemonID, env)
}

func (t hubTransport) Release(daemonID, _ string) {
	t.hub.Release(daemonID)
}

type hubPlacement struct {
	hub DaemonHub
}

func (p hubPlacement) PickHealthy(
	runtimeType runtimev1.RuntimeType,
	runtimeName string,
	labels map[string]string,
) (string, bool) {
	return p.hub.PickHealthy(runtimeType, runtimeName, labels)
}

func (p hubPlacement) Reserve(daemonID, _ string, _ time.Duration) bool {
	return p.hub.Reserve(daemonID)
}

func (p hubPlacement) Release(daemonID, _ string) {
	p.hub.Release(daemonID)
}

func (p hubPlacement) Disconnect(daemonID string) {
	p.hub.Disconnect(daemonID)
}

var _ ports.RuntimeTransport = hubTransport{}
var _ ports.RuntimePlacement = hubPlacement{}
