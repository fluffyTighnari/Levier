// Middleware — internal/control
//
// Control scheduler and event tests.
//
// Copyright 2026 The Middleware Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

type fakeRuns struct {
	mu           sync.Mutex
	run          runstore.Run
	assignCalls  int
	assignedTo   string
	statuses     []string
	events       []runstore.Event
	appendResult runstore.AppendResult
}

func (f *fakeRuns) GetRun(context.Context, string) (runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.run, nil
}

func (f *fakeRuns) AssignRun(_ context.Context, assignment runstore.Assignment) (runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignCalls++
	f.assignedTo = assignment.DaemonID
	f.run.Status = "assigned"
	f.run.Attempt = assignment.Attempt
	f.run.LeaseEpoch = assignment.LeaseEpoch
	f.run.AssignedDaemonID = assignment.DaemonID
	return f.run, nil
}

func (f *fakeRuns) Transition(_ context.Context, transition runstore.Transition) (runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if transition.ExpectedStatus != f.run.Status ||
		transition.ExpectedAttempt != f.run.Attempt ||
		transition.ExpectedLeaseEpoch != f.run.LeaseEpoch {
		return runstore.Run{}, runstore.ErrRunNotAssignable
	}
	f.statuses = append(f.statuses, transition.ToStatus)
	f.run.Status = transition.ToStatus
	return f.run, nil
}

func (f *fakeRuns) AppendEvent(_ context.Context, event runstore.Event) (runstore.AppendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return f.appendResult, nil
}

type fakeHub struct {
	mu        sync.Mutex
	daemonID  string
	sent      []*runtimev1.Envelope
	onSend    func(*runtimev1.Envelope)
	pickCalls int
}

func (f *fakeHub) PickHealthy(runtimev1.RuntimeType, string, map[string]string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pickCalls++
	if f.daemonID == "" {
		return "", false
	}
	return f.daemonID, true
}

func (f *fakeHub) Send(daemonID string, env *runtimev1.Envelope) error {
	f.mu.Lock()
	f.sent = append(f.sent, env)
	callback := f.onSend
	f.mu.Unlock()
	if callback != nil {
		callback(env)
	}
	return nil
}

func newRun() runstore.Run {
	return runstore.Run{
		RunID:         "run-1",
		AppID:         "app-1",
		Runtime:       "normal",
		Strategy:      "echo",
		Params:        json.RawMessage(`{"max_steps":8}`),
		Input:         json.RawMessage(`[{"role":"user","content":"hi"}]`),
		Model:         json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
		Status:        "queued",
		RunTimeoutMS:  1800000,
		ToolTimeoutMS: 60000,
		TraceID:       "trace-1",
		LeaseEpoch:    0,
	}
}

func testTask() workflow.Task {
	return workflow.Task{RunID: "run-1", Type: workflow.TaskDispatch, Attempt: 1}
}

func TestHandleDispatchAndAcceptedAck(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "daemon-1"}
	ctrl := New(runs, hub, Config{AckTimeout: time.Second, Logger: slog.Default()})
	hub.onSend = func(env *runtimev1.Envelope) {
		go ctrl.HandleEnvelope("daemon-1", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: true},
			},
		})
	}

	if err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if runs.assignedTo != "daemon-1" || runs.assignCalls != 1 {
		t.Fatalf("assignment = %q calls=%d", runs.assignedTo, runs.assignCalls)
	}
	if len(hub.sent) != 1 {
		t.Fatalf("sent = %d", len(hub.sent))
	}
	assign := hub.sent[0].GetTaskAssign()
	if assign.GetRunId() != "run-1" || assign.GetStrategy() != "echo" {
		t.Fatalf("assign = %+v", assign)
	}
	if assign.GetModel().GetProvider() != "anthropic" {
		t.Fatalf("model = %+v", assign.GetModel())
	}
	if len(assign.GetInput()) != 1 || assign.GetInput()[0].GetContent() != "hi" {
		t.Fatalf("input = %+v", assign.GetInput())
	}
	if assign.GetTimeouts().GetRunTimeoutMs() != 1800000 {
		t.Fatalf("timeouts = %+v", assign.GetTimeouts())
	}
}

func TestHandleNoDaemonRetries(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	ctrl := New(runs, &fakeHub{}, Config{RetryDelay: 3 * time.Second})
	err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{})
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
	delay, ok := workflow.RetryAfterDuration(err)
	if !ok || delay != 3*time.Second {
		t.Fatalf("retry = %v, %v; err=%v", delay, ok, err)
	}
	if runs.assignCalls != 0 {
		t.Fatalf("assign calls = %d", runs.assignCalls)
	}
}

func TestHandleRejectedAckRetries(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "daemon-1"}
	ctrl := New(runs, hub, Config{AckTimeout: time.Second})
	hub.onSend = func(env *runtimev1.Envelope) {
		go ctrl.HandleEnvelope("daemon-1", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: false, Reason: "busy"},
			},
		})
	}
	err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{})
	if err == nil || !errors.Is(err, errNoDaemon) && err.Error() != "busy" {
		if _, ok := workflow.RetryAfterDuration(err); !ok {
			t.Fatalf("Handle() error = %v", err)
		}
	}
}

func TestHandleAckTimeoutRetries(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "daemon-1"}
	ctrl := New(runs, hub, Config{AckTimeout: 10 * time.Millisecond, RetryDelay: time.Second})
	err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{})
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
	delay, ok := workflow.RetryAfterDuration(err)
	if !ok || delay != time.Second {
		t.Fatalf("retry = %v, %v; err=%v", delay, ok, err)
	}
}

func TestEventUpdatesRunStatus(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "assigned"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	ctrl := New(runs, &fakeHub{}, Config{})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	data, _ := structpb.NewStruct(map[string]any{"status": "running"})
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: protocol.EventSourceDaemon,
		Data:   data,
	}
	env, err := protocol.NewRun("run-1", 1, 1, event)
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	ctrl.HandleEnvelope("daemon-1", env)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runs.mu.Lock()
		statuses := append([]string(nil), runs.statuses...)
		runs.mu.Unlock()
		if len(statuses) == 1 && statuses[0] == "running" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("statuses = %v, want running", runs.statuses)
}

func TestControlEventCounters(t *testing.T) {
	runs := &fakeRuns{
		run: newRun(),
		appendResult: runstore.AppendResult{
			Inserted: true,
			Gap:      true,
			GapSize:  17,
		},
	}
	runs.run.Status = "assigned"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	ctrl := New(runs, &fakeHub{}, Config{})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	data, _ := structpb.NewStruct(map[string]any{"status": "running"})
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: protocol.EventSourceDaemon,
		Data:   data,
	}
	env, _ := protocol.NewRun("run-1", 1, 1, event)
	ctrl.HandleEnvelope("daemon-1", env)

	ephemeral := &runtimev1.Event{
		Seq:       0,
		Type:      runtimev1.EventType_MESSAGE_DELTA,
		Ephemeral: true,
		Source:    protocol.EventSourceDaemon,
		Data:      data,
	}
	ephemeralEnv, _ := protocol.NewRun("run-1", 1, 1, ephemeral)
	ctrl.HandleEnvelope("daemon-1", ephemeralEnv)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ctrl.GapEvents() == 1 && ctrl.EphemeralEvents() == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("gap=%d ephemeral=%d", ctrl.GapEvents(), ctrl.EphemeralEvents())
}

func TestStaleEpochEventIgnored(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "running"
	runs.run.Attempt = 2
	runs.run.LeaseEpoch = 2
	ctrl := New(runs, &fakeHub{}, Config{})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	data, _ := structpb.NewStruct(map[string]any{"status": "completed"})
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: protocol.EventSourceDaemon,
		Data:   data,
	}
	env, _ := protocol.NewRun("run-1", 1, 1, event)
	ctrl.HandleEnvelope("daemon-1", env)
	time.Sleep(20 * time.Millisecond)
	runs.mu.Lock()
	defer runs.mu.Unlock()
	if len(runs.events) != 0 || len(runs.statuses) != 0 {
		t.Fatalf("stale event applied: events=%d statuses=%v", len(runs.events), runs.statuses)
	}
}
