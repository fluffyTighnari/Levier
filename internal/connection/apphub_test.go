// Levier — internal/connection
//
// AppHub fanout and subscription tests.
//
// Copyright 2026 The Levier Authors.
package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestCallLocalToolAndResolve(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	defer hub.CloseConn(conn)

	resultCh := make(chan LocalToolResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-1", ToolName: "search", ArgumentsJSON: `{"q":"x"}`, TimeoutMS: 1000,
		})
		resultCh <- result
		errCh <- err
	}()

	if !waitSinkCount(t, sink, 1) {
		t.Fatal("local tool call was not sent")
	}
	if !hub.ResolveLocalTool("app-1", LocalToolResult{
		CorrelationID: lastCorrelationID(t, sink),
		InvocationID:  "inv-1", OK: true, ResultJSON: `{"ok":true}`,
	}) {
		t.Fatal("ResolveLocalTool() = false")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	result := <-resultCh
	if !result.OK || result.ResultJSON != `{"ok":true}` {
		t.Fatalf("result = %+v", result)
	}
	if hub.ResolveLocalTool("app-1", LocalToolResult{InvocationID: "inv-1", OK: true}) {
		t.Fatal("duplicate result was accepted")
	}
	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{InvocationID: "inv-1"}); got != LocalToolDuplicate {
		t.Fatalf("duplicate resolution = %v", got)
	}
	if hub.PendingLocalTools("app-1") != 0 {
		t.Fatal("pending local tool was not removed")
	}
}

func TestLocalToolTimeoutAndDisconnect(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)

	if _, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
		InvocationID: "inv-timeout", ToolName: "search", TimeoutMS: 10,
	}); !errors.Is(err, ErrLocalToolTimeout) {
		t.Fatalf("timeout error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-disconnect", ToolName: "search", TimeoutMS: 1000,
		})
		errCh <- err
	}()
	if !waitSinkCount(t, sink, 2) {
		t.Fatal("second local tool call was not sent")
	}
	hub.CloseConn(conn)
	if err := <-errCh; !errors.Is(err, ErrAppUnreachable) {
		t.Fatalf("disconnect error = %v, want ErrAppUnreachable", err)
	}
}

func TestCallLocalToolTargetsOneConnection(t *testing.T) {
	hub := NewAppHub()
	firstSink := &recordingSink{}
	secondSink := &recordingSink{}
	first := hub.Register("app-1", firstSink)
	second := hub.Register("app-1", secondSink)
	defer hub.CloseConn(first)
	defer hub.CloseConn(second)

	resultCh := make(chan LocalToolResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-directed", ToolName: "search", TimeoutMS: 1000,
		})
		resultCh <- result
		errCh <- err
	}()

	deadline := time.Now().Add(time.Second)
	for firstSink.count()+secondSink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	firstCount, secondCount := firstSink.count(), secondSink.count()
	if firstCount+secondCount != 1 {
		t.Fatalf("local tool call fanout = %d, want exactly one", firstCount+secondCount)
	}
	target := first
	targetSink := firstSink
	if secondCount == 1 {
		target = second
		targetSink = secondSink
	}
	if !hub.ResolveLocalTool("app-1", LocalToolResult{
		CorrelationID: lastCorrelationID(t, targetSink),
		InvocationID:  "inv-directed",
		ConnectionID:  target.ConnectionID(),
		OK:            true,
		ResultJSON:    "3",
	}) {
		t.Fatal("target connection result was rejected")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	if result := <-resultCh; !result.OK || result.ResultJSON != "3" {
		t.Fatalf("result = %+v", result)
	}
}

func TestLocalToolResultFromWrongConnectionIsRejected(t *testing.T) {
	hub := NewAppHub()
	firstSink := &recordingSink{}
	secondSink := &recordingSink{}
	first := hub.Register("app-1", firstSink)
	second := hub.Register("app-1", secondSink)
	defer hub.CloseConn(first)
	defer hub.CloseConn(second)

	go func() {
		_, _ = hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-wrong-conn", ToolName: "search", TimeoutMS: 2000,
		})
	}()
	deadline := time.Now().Add(time.Second)
	for firstSink.count()+secondSink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	firstCount, secondCount := firstSink.count(), secondSink.count()
	target, targetSink, other := first, firstSink, second
	if secondCount == 1 {
		target, targetSink, other = second, secondSink, first
	}
	if firstCount+secondCount != 1 {
		t.Fatalf("local tool call fanout = %d, want exactly one", firstCount+secondCount)
	}
	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{
		CorrelationID: lastCorrelationID(t, targetSink),
		InvocationID:  "inv-wrong-conn",
		ConnectionID:  other.ConnectionID(),
		OK:            true,
	}); got != LocalToolForbidden {
		t.Fatalf("wrong-connection result = %v, want LocalToolForbidden", got)
	}
	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{
		CorrelationID: lastCorrelationID(t, targetSink),
		InvocationID:  "inv-wrong-conn",
		ConnectionID:  target.ConnectionID(),
		OK:            true,
	}); got != LocalToolResolved {
		t.Fatalf("target result = %v, want LocalToolResolved", got)
	}
}

func TestCancelLocalToolsByRun(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	defer hub.CloseConn(conn)

	resultCh := make(chan LocalToolResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-cancel", RunID: "run-1", ToolName: "search", TimeoutMS: 1000,
		})
		resultCh <- result
		errCh <- err
	}()
	if !waitSinkCount(t, sink, 1) {
		t.Fatal("local tool call was not sent")
	}
	hub.CancelLocalTools("app-1", "run-1", "RUN_CANCELLED", "cancelled")
	if err := <-errCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	result := <-resultCh
	if result.Error == nil || result.Error.Code != "RUN_CANCELLED" {
		t.Fatalf("result = %+v", result)
	}
}

func TestLocalToolNoAppConnection(t *testing.T) {
	hub := NewAppHub()
	if _, err := hub.CallLocalTool(context.Background(), "missing", LocalToolRequest{
		InvocationID: "inv-none", ToolName: "search", TimeoutMS: 10,
	}); !errors.Is(err, ErrAppUnreachable) {
		t.Fatalf("error = %v, want ErrAppUnreachable", err)
	}
}

func waitSinkCount(t *testing.T, sink *recordingSink, want int) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sink.count() >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
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

func TestAppConnReplayBuffersAndDeduplicates(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	defer hub.CloseConn(conn)
	conn.BeginReplay("run-1", 0, 0)
	conn.Subscribe("run-1")

	hub.Publish("app-1", "run-1", eventPayload(1, 1))
	hub.Publish("app-1", "run-1", eventPayload(1, 2))
	hub.Publish("app-1", "run-1", eventPayload(1, 2))
	if sink.count() != 0 {
		t.Fatalf("live events escaped the replay buffer: %d", sink.count())
	}

	if err := conn.SendReplay("run-1", 1, 1, eventPayload(1, 1)); err != nil {
		t.Fatalf("SendReplay(1) error = %v", err)
	}
	if err := conn.SendReplay("run-1", 1, 2, eventPayload(1, 2)); err != nil {
		t.Fatalf("SendReplay(2) error = %v", err)
	}
	if err := conn.EndReplay("run-1", 1, 2); err != nil {
		t.Fatalf("EndReplay() error = %v", err)
	}

	hub.Publish("app-1", "run-1", eventPayload(1, 2))
	hub.Publish("app-1", "run-1", eventPayload(1, 3))

	want := []string{`"seq":1`, `"seq":2`, `"seq":3`}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.payloads) != len(want) {
		t.Fatalf("payloads = %d, want %d", len(sink.payloads), len(want))
	}
	for i, fragment := range want {
		if !strings.Contains(string(sink.payloads[i]), fragment) {
			t.Fatalf("payload[%d] = %s", i, sink.payloads[i])
		}
	}
}

func eventPayload(attempt int32, seq int64) []byte {
	return []byte(fmt.Sprintf(`{"type":"event","attempt":%d,"seq":%d}`, attempt, seq))
}

func lastCorrelationID(t *testing.T, sink *recordingSink) string {
	t.Helper()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.payloads) == 0 {
		t.Fatal("no payload recorded")
	}
	var message struct {
		MsgID string `json:"msg_id"`
	}
	if err := json.Unmarshal(sink.payloads[len(sink.payloads)-1], &message); err != nil {
		t.Fatalf("decode local tool call: %v", err)
	}
	return message.MsgID
}

func TestResolveLocalToolValidation(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	defer hub.CloseConn(conn)

	resultCh := make(chan error, 1)
	go func() {
		_, err := hub.CallLocalTool(context.Background(), "app-1", LocalToolRequest{
			InvocationID: "inv-1", ToolName: "search", ArgumentsJSON: `{}`, TimeoutMS: 1000,
		})
		resultCh <- err
	}()
	if !waitSinkCount(t, sink, 1) {
		t.Fatal("local tool call was not sent")
	}
	correlation := lastCorrelationID(t, sink)

	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{
		CorrelationID: "wrong", InvocationID: "inv-1", OK: true,
	}); got != LocalToolCorrelationMismatch {
		t.Fatalf("mismatch resolution = %v", got)
	}
	if got := hub.ResolveLocalToolChecked("app-2", LocalToolResult{
		CorrelationID: correlation, InvocationID: "inv-1", OK: true,
	}); got != LocalToolForbidden {
		t.Fatalf("forbidden resolution = %v", got)
	}
	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{
		CorrelationID: correlation, InvocationID: "missing", OK: true,
	}); got != LocalToolUnknown {
		t.Fatalf("unknown resolution = %v", got)
	}
	if got := hub.ResolveLocalToolChecked("app-1", LocalToolResult{
		CorrelationID: correlation, InvocationID: "inv-1", OK: true, ResultJSON: "3",
	}); got != LocalToolResolved {
		t.Fatalf("resolved resolution = %v", got)
	}
	if err := <-resultCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	cached, ok := hub.CachedLocalToolResult("app-1", "inv-1")
	if !ok || cached.ResultJSON != "3" || cached.CorrelationID != correlation {
		t.Fatalf("cached result = %+v, ok=%v", cached, ok)
	}
	if _, ok := hub.CachedLocalToolResult("app-2", "inv-1"); ok {
		t.Fatal("cached result leaked to another app")
	}
}

func replayPayload(seq int64) []byte {
	return []byte(fmt.Sprintf(`{"type":"event","run_id":"run-1","attempt":1,"seq":%d}`, seq))
}

func TestReplayBufferOverflowIsBounded(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	defer hub.CloseConn(conn)
	conn.BeginReplay("run-1", 0, 0)

	for seq := int64(1); seq <= maxReplayPayloads; seq++ {
		if err := conn.SendRun("run-1", replayPayload(seq)); err != nil {
			t.Fatalf("SendRun(%d) error = %v", seq, err)
		}
	}
	if err := conn.SendRun("run-1", replayPayload(maxReplayPayloads+1)); !errors.Is(err, ErrReplayOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
	if err := conn.EndReplay("run-1", 0, 0); !errors.Is(err, ErrReplayOverflow) {
		t.Fatalf("EndReplay() error = %v", err)
	}
}

func TestPublishClosesOverflowedReplayConnection(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	conn.Subscribe("run-1")
	conn.BeginReplay("run-1", 0, 0)

	for seq := int64(1); seq <= maxReplayPayloads+1; seq++ {
		hub.Publish("app-1", "run-1", replayPayload(seq))
	}
	if hub.Count("app-1") != 0 {
		t.Fatalf("connection count = %d, want 0", hub.Count("app-1"))
	}
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	if !closed {
		t.Fatal("overflowed sink was not closed")
	}
}

func TestBroadcastRespectsClosedConnection(t *testing.T) {
	hub := NewAppHub()
	sink := &recordingSink{}
	conn := hub.Register("app-1", sink)
	conn.close()
	if hub.Broadcast("app-1", []byte(`{"type":"ping"}`)) {
		t.Fatal("Broadcast() reported delivery to a closed connection")
	}
	if sink.count() != 0 {
		t.Fatalf("closed sink received %d payloads", sink.count())
	}
}

func TestAppHubStats(t *testing.T) {
	hub := NewAppHub()
	first := hub.Register("app-1", &recordingSink{})
	second := hub.Register("app-1", &recordingSink{})
	hub.Register("app-2", &recordingSink{})
	stats := hub.Stats()
	if stats.Apps != 2 || stats.Connections != 3 {
		t.Fatalf("Stats() = %+v", stats)
	}
	hub.CloseConn(first)
	hub.CloseConn(second)
	if stats := hub.Stats(); stats.Apps != 1 || stats.Connections != 1 {
		t.Fatalf("Stats() after close = %+v", stats)
	}
}
