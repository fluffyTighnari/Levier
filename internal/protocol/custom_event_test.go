// Levier - custom event protocol validation tests.
// Copyright 2026 The Levier Authors.

package protocol

import (
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

func TestCustomEventRequiresName(t *testing.T) {
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_CUSTOM,
		Source: EventSourceDaemon,
	}
	if _, err := NewRun("run-1", 1, 1, event); err == nil {
		t.Fatal("custom event without name was accepted")
	}

	event.Name = "graph.node.started"
	event.SchemaVersion = "1"
	if _, err := NewRun("run-1", 1, 1, event); err != nil {
		t.Fatalf("custom event rejected: %v", err)
	}
}

func TestFixedEventRejectsCustomMetadata(t *testing.T) {
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_MESSAGE_COMPLETED,
		Source: EventSourceDaemon,
		Name:   "graph.node.started",
	}
	if _, err := NewRun("run-1", 1, 1, event); err == nil {
		t.Fatal("fixed event with custom metadata was accepted")
	}
}
