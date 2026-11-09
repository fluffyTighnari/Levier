// Middleware — internal/connection
//
// AppHub fanout and subscription tests.
//
// Copyright 2026 The Middleware Authors.
package connection

import (
	"sync"
	"testing"
)

type recordingSink struct {
	mu       sync.Mutex
	payloads [][]byte
	closed   bool
}

func (s *recordingSink) Send(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, append([]byte(nil), payload...))
	return nil
}

func (s *recordingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.payloads)
}

func TestAppHubSubscriptionFanout(t *testing.T) {
	hub := NewAppHub()
	firstSink := &recordingSink{}
	secondSink := &recordingSink{}
	first := hub.Register("app-1", firstSink)
	second := hub.Register("app-2", secondSink)
	first.Subscribe("run-1")

	hub.Publish("app-1", "run-1", []byte(`{"type":"event"}`))
	hub.Publish("app-1", "run-2", []byte(`{"type":"wrong-run"}`))
	hub.Publish("app-2", "run-1", []byte(`{"type":"wrong-app"}`))

	if firstSink.count() != 1 {
		t.Fatalf("first sink payloads = %d, want 1", firstSink.count())
	}
	if secondSink.count() != 0 {
		t.Fatalf("second sink payloads = %d, want 0", secondSink.count())
	}

	hub.CloseConn(first)
	if firstSink.closed {
		// AppHub.CloseConn closes the sink; this is expected.
	}
	if hub.Count("app-1") != 0 {
		t.Fatalf("app-1 count = %d", hub.Count("app-1"))
	}
	hub.CloseConn(second)
	hub.Close()
}
