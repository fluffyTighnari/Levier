// Middleware — internal/control
//
// Control scheduler and event tests.
//
// Copyright 2026 The Middleware Authors.
package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/register"
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
	active       []runstore.Run
	queued       []runstore.Run
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
	if runstore.TerminalStatus(f.run.Status) || transition.ExpectedStatus != f.run.Status ||
		transition.ExpectedAttempt != f.run.Attempt ||
		transition.ExpectedLeaseEpoch != f.run.LeaseEpoch {
		return runstore.Run{}, runstore.ErrRunNotAssignable
	}
	f.statuses = append(f.statuses, transition.ToStatus)
	f.run.Status = transition.ToStatus
	return f.run, nil
}

func (f *fakeRuns) ListActiveRunsForDaemon(context.Context, string) ([]runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active != nil {
		return append([]runstore.Run(nil), f.active...), nil
	}
	return []runstore.Run{f.run}, nil
}

func (f *fakeRuns) ListActiveRuns(context.Context) ([]runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active != nil {
		return append([]runstore.Run(nil), f.active...), nil
	}
	return []runstore.Run{f.run}, nil
}

func (f *fakeRuns) ListQueuedRuns(context.Context) ([]runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.queued != nil {
		return append([]runstore.Run(nil), f.queued...), nil
	}
	if f.run.Status == "queued" {
		return []runstore.Run{f.run}, nil
	}
	return nil, nil
}

func (f *fakeRuns) RequeueRun(_ context.Context, _ string) (runstore.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.Status = "queued"
	f.run.AssignedDaemonID = ""
	f.run.LeaseEpoch++
	return f.run, nil
}

func (f *fakeRuns) AppendEvent(_ context.Context, event runstore.Event) (runstore.AppendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return f.appendResult, nil
}

type fakeHub struct {
	mu           sync.Mutex
	daemonID     string
	sent         []*runtimev1.Envelope
	onSend       func(*runtimev1.Envelope)
	pickCalls    int
	reserveCalls int
	releaseCalls int
	released     []string
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

func (f *fakeHub) Reserve(string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserveCalls++
	return true
}

func (f *fakeHub) Release(daemonID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	f.released = append(f.released, daemonID)
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

type fakeMaterials struct{}

func (fakeMaterials) AssembleTaskMaterials(context.Context, string) (register.TaskMaterials, error) {
	return register.TaskMaterials{
		Tools: []register.ToolMaterial{{
			Name: "search", Description: "search", SchemaJSON: `{"type":"object"}`, Scope: "local",
		}},
		MCPBindings: []register.MCPMaterial{{
			ServerName: "github", Transport: "stdio", Command: "mcp-github",
			AllowedTools: []string{"search"}, Namespace: "mcp__github__",
		}},
	}, nil
}

type fakeScheduler struct {
	request PlacementRequest
	id      string
	ok      bool
}

func (f *fakeScheduler) Pick(request PlacementRequest) (string, bool) {
	f.request = request
	return f.id, f.ok
}

type fakeInvoker struct {
	called bool
	result *runtimev1.ToolCallResult
}

func (f *fakeInvoker) Invoke(_ context.Context, runID, appID string, request *runtimev1.ToolCallRequest) *runtimev1.ToolCallResult {
	f.called = true
	return f.result
}

type fakeTaskCanceller struct {
	calls        int
	enqueueCalls int
	runID        string
}

func (f *fakeTaskCanceller) CancelRun(_ context.Context, runID, _ string) (int, error) {
	f.calls++
	f.runID = runID
	return 1, nil
}

func (f *fakeTaskCanceller) Enqueue(_ context.Context, task workflow.Task) (workflow.Task, error) {
	f.enqueueCalls++
	f.runID = task.RunID
	return task, nil
}

type fakeLocalTools struct {
	calls int
	runID string
	code  string
}

func (f *fakeLocalTools) CancelLocalTools(_, runID, code, _ string) {
	f.calls++
	f.runID = runID
	f.code = code
}

type sequenceScheduler struct {
	ids      []string
	requests []PlacementRequest
}

func (s *sequenceScheduler) Pick(request PlacementRequest) (string, bool) {
	s.requests = append(s.requests, request)
	for _, id := range s.ids {
		if _, excluded := request.ExcludeDaemonIDs[id]; excluded {
			continue
		}
		return id, true
	}
	return "", false
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

func TestHandleIncludesAssembledMaterials(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "daemon-1"}
	ctrl := New(runs, hub, Config{AckTimeout: time.Second}, fakeMaterials{})
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
	assign := hub.sent[0].GetTaskAssign()
	if len(assign.GetTools()) != 1 || assign.GetTools()[0].GetScope() != runtimev1.ToolScope_LOCAL {
		t.Fatalf("tools = %+v", assign.GetTools())
	}
	if len(assign.GetMcpBindings()) != 1 || assign.GetMcpBindings()[0].GetNamespace() != "mcp__github__" {
		t.Fatalf("mcp bindings = %+v", assign.GetMcpBindings())
	}
}

func TestHandleUsesLongtermStickyScheduler(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Runtime = "longterm"
	runs.run.AssignedDaemonID = "daemon-old"
	hub := &fakeHub{daemonID: "daemon-old"}
	scheduler := &fakeScheduler{id: "daemon-old", ok: true}
	ctrl := New(runs, hub, Config{AckTimeout: time.Second, Scheduler: scheduler})
	hub.onSend = func(env *runtimev1.Envelope) {
		go ctrl.HandleEnvelope("daemon-old", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: true},
			},
		})
	}
	if err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if scheduler.request.StickyDaemonID != "daemon-old" {
		t.Fatalf("sticky daemon = %q", scheduler.request.StickyDaemonID)
	}
	if scheduler.request.Runtime != runtimev1.RuntimeType_LONGTERM {
		t.Fatalf("runtime = %v", scheduler.request.Runtime)
	}
}

func TestHandleRetriesNextCandidateOnRejection(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "d1"}
	scheduler := &sequenceScheduler{ids: []string{"d1", "d2"}}
	ctrl := New(runs, hub, Config{
		AckTimeout:           time.Second,
		MaxPlacementAttempts: 3,
		Scheduler:            scheduler,
	})
	var calls int
	hub.onSend = func(env *runtimev1.Envelope) {
		calls++
		accepted := calls > 1
		reason := "busy"
		if accepted {
			reason = ""
		}
		go ctrl.HandleEnvelope("", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: accepted, Reason: reason},
			},
		})
	}
	if err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if runs.assignedTo != "d2" {
		t.Fatalf("assigned to = %q, want d2", runs.assignedTo)
	}
	if hub.releaseCalls != 1 {
		t.Fatalf("releases = %d, want 1", hub.releaseCalls)
	}
	if len(scheduler.requests) != 2 {
		t.Fatalf("scheduler requests = %d", len(scheduler.requests))
	}
	if _, excluded := scheduler.requests[1].ExcludeDaemonIDs["d1"]; !excluded {
		t.Fatal("rejected daemon was not excluded")
	}
}

func TestHandleRetriesNextCandidateOnAckTimeout(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{daemonID: "d1"}
	scheduler := &sequenceScheduler{ids: []string{"d1", "d2"}}
	ctrl := New(runs, hub, Config{
		AckTimeout:           10 * time.Millisecond,
		MaxPlacementAttempts: 3,
		Scheduler:            scheduler,
	})
	var calls int
	hub.onSend = func(env *runtimev1.Envelope) {
		calls++
		if calls == 1 {
			return
		}
		go ctrl.HandleEnvelope("", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: true},
			},
		})
	}
	if err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if runs.assignedTo != "d2" || runs.run.Attempt != 2 || runs.run.LeaseEpoch != 2 {
		t.Fatalf("assignment = %+v", runs.run)
	}
	if hub.releaseCalls != 1 {
		t.Fatalf("releases = %d, want 1", hub.releaseCalls)
	}
	if len(scheduler.requests) != 2 {
		t.Fatalf("scheduler requests = %d", len(scheduler.requests))
	}
	if _, excluded := scheduler.requests[1].ExcludeDaemonIDs["d1"]; !excluded {
		t.Fatal("timed-out daemon was not excluded")
	}
}

func TestHandleToolCallRoutesAndResponds(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{}
	invocationID := id.New()
	invoker := &fakeInvoker{result: &runtimev1.ToolCallResult{
		InvocationId: invocationID, Ok: true, ResultJson: "3",
	}}
	ctrl := New(runs, hub, Config{Tools: invoker})
	request := &runtimev1.ToolCallRequest{
		InvocationId: invocationID, ToolName: "search", ArgumentsJson: "{}", TimeoutMs: 1000,
	}
	env, err := protocol.NewRun("run-1", 1, 1, request)
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := ctrl.HandleToolCall("d1", env); err != nil {
		t.Fatalf("HandleToolCall() error = %v", err)
	}
	if !invoker.called || len(hub.sent) != 1 {
		t.Fatalf("called=%v sent=%d", invoker.called, len(hub.sent))
	}
	if hub.sent[0].GetToolCallResult().GetResultJson() != "3" || hub.sent[0].GetCorrelationId() != env.GetMsgId() {
		t.Fatalf("response = %+v", hub.sent[0])
	}
}

func TestReconcileDaemonCases(t *testing.T) {
	info := connection.DaemonInfo{DaemonID: "d1", RunningRuns: []connection.RunRef{
		{RunID: "run-1", LeaseEpoch: 1},
	}}

	t.Run("same epoch no action", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "running"
		runs.run.Attempt = 1
		runs.run.LeaseEpoch = 1
		runs.run.AssignedDaemonID = "d1"
		hub := &fakeHub{}
		ctrl := New(runs, hub, Config{})
		ctrl.ReconcileDaemon(info)
		if len(hub.sent) != 0 {
			t.Fatalf("reconcile sent %+v", hub.sent)
		}
	})

	t.Run("missing run requeues", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "running"
		runs.run.Attempt = 1
		runs.run.LeaseEpoch = 1
		runs.run.AssignedDaemonID = "d1"
		tasks := &fakeTaskCanceller{}
		ctrl := New(runs, &fakeHub{}, Config{Tasks: tasks})
		ctrl.ReconcileDaemon(connection.DaemonInfo{DaemonID: "d1"})
		current, _ := runs.GetRun(context.Background(), "run-1")
		if current.Status != "queued" || current.Attempt != 1 || current.LeaseEpoch != 2 {
			t.Fatalf("requeued = %+v", current)
		}
		if tasks.enqueueCalls != 1 {
			t.Fatalf("retry enqueue calls = %d", tasks.enqueueCalls)
		}
	})

	t.Run("queued run is rescheduled on reconnect", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "queued"
		tasks := &fakeTaskCanceller{}
		ctrl := New(runs, &fakeHub{}, Config{Tasks: tasks})
		ctrl.ReconcileDaemon(connection.DaemonInfo{
			DaemonID:    "d1",
			RuntimeType: runtimev1.RuntimeType_NORMAL,
		})
		if tasks.enqueueCalls != 1 || tasks.runID != "run-1" {
			t.Fatalf("queued run was not rescheduled: %+v", tasks)
		}
	})

	t.Run("queued run for another runtime is left alone", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "queued"
		tasks := &fakeTaskCanceller{}
		ctrl := New(runs, &fakeHub{}, Config{Tasks: tasks})
		ctrl.ReconcileDaemon(connection.DaemonInfo{
			DaemonID:    "d1",
			RuntimeType: runtimev1.RuntimeType_SANDBOX,
		})
		if tasks.enqueueCalls != 0 {
			t.Fatalf("queued run scheduled for a mismatched runtime")
		}
	})

	t.Run("terminal run is cancelled on daemon", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "completed"
		runs.run.Attempt = 1
		runs.run.LeaseEpoch = 1
		runs.run.AssignedDaemonID = "d1"
		hub := &fakeHub{}
		ctrl := New(runs, hub, Config{})
		ctrl.ReconcileDaemon(info)
		if len(hub.sent) != 1 || hub.sent[0].GetReconcile() == nil {
			t.Fatalf("reconcile = %+v", hub.sent)
		}
	})

	t.Run("epoch mismatch cancels old execution", func(t *testing.T) {
		runs := &fakeRuns{run: newRun()}
		runs.run.Status = "running"
		runs.run.Attempt = 2
		runs.run.LeaseEpoch = 2
		runs.run.AssignedDaemonID = "d1"
		hub := &fakeHub{}
		ctrl := New(runs, hub, Config{})
		ctrl.ReconcileDaemon(connection.DaemonInfo{
			DaemonID:    "d1",
			RunningRuns: []connection.RunRef{{RunID: "run-1", LeaseEpoch: 1}},
		})
		if len(hub.sent) != 1 || hub.sent[0].GetReconcile() == nil {
			t.Fatalf("reconcile = %+v", hub.sent)
		}
	})
}

func TestHandleDaemonOfflineRequeues(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	tasks := &fakeTaskCanceller{}
	ctrl := New(runs, &fakeHub{}, Config{Tasks: tasks})
	ctrl.HandleDaemonOffline("d1")
	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != "queued" || current.Attempt != 1 {
		t.Fatalf("run = %+v", current)
	}
	if tasks.enqueueCalls != 1 {
		t.Fatalf("enqueue calls = %d", tasks.enqueueCalls)
	}
}

func TestCancelRunSendsCancelAndFailsTools(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	hub := &fakeHub{}
	tasks := &fakeTaskCanceller{}
	localTools := &fakeLocalTools{}
	ctrl := New(runs, hub, Config{Tasks: tasks, LocalTools: localTools})

	cancelled, err := ctrl.CancelRun(context.Background(), "run-1", "user")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if cancelled.Status != runstore.StatusCancelled {
		t.Fatalf("status = %q", cancelled.Status)
	}
	if len(hub.sent) != 1 || hub.sent[0].GetCancel() == nil {
		t.Fatalf("cancel was not sent: %+v", hub.sent)
	}
	if tasks.calls != 1 || tasks.runID != "run-1" {
		t.Fatalf("tasks = %+v", tasks)
	}
	if localTools.calls != 1 || localTools.code != "RUN_CANCELLED" {
		t.Fatalf("local tools = %+v", localTools)
	}
	if hub.releaseCalls != 1 {
		t.Fatalf("release calls = %d", hub.releaseCalls)
	}
}

func TestCancelRunWinsOverLateCompletion(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	ctrl := New(runs, &fakeHub{}, Config{})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	if _, err := ctrl.CancelRun(context.Background(), "run-1", "user"); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	data, _ := structpb.NewStruct(map[string]any{"status": "completed"})
	event := &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon, Data: data,
	}
	env, _ := protocol.NewRun("run-1", 1, 1, event)
	ctrl.HandleEnvelope("d1", env)
	time.Sleep(20 * time.Millisecond)

	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != runstore.StatusCancelled {
		t.Fatalf("late completion changed status to %q", current.Status)
	}
}

func TestRunTimeoutTerminal(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	hub := &fakeHub{}
	localTools := &fakeLocalTools{}
	ctrl := New(runs, hub, Config{LocalTools: localTools})
	task := testTask()
	task.Type = workflow.TaskTimeout

	if err := ctrl.Handle(context.Background(), task, workflow.Lease{}); err != nil {
		t.Fatalf("Handle(timeout) error = %v", err)
	}
	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != runstore.StatusTimeout {
		t.Fatalf("status = %q, want timeout", current.Status)
	}
	if len(hub.sent) != 1 || hub.sent[0].GetCancel() == nil {
		t.Fatal("timeout did not send cancel")
	}
	if localTools.code != "RUN_TIMEOUT" {
		t.Fatalf("local tool code = %q", localTools.code)
	}
}

type fakePublisher struct {
	mu       sync.Mutex
	statuses []string
}

func (f *fakePublisher) Publish(_ string, _ string, payload []byte) {
	var message struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(payload, &message)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, message.Status)
}

func TestTerminalEventAfterRequeueDoesNotPublish(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	hub := &fakeHub{}
	publisher := &fakePublisher{}
	ctrl := New(runs, hub, Config{Publisher: publisher})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	// A daemon-offline requeue wins the race before the terminal event lands.
	if _, err := runs.RequeueRun(context.Background(), "run-1"); err != nil {
		t.Fatalf("RequeueRun() error = %v", err)
	}

	data, _ := structpb.NewStruct(map[string]any{"status": "cancelled"})
	event := &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon, Data: data,
	}
	env, _ := protocol.NewRun("run-1", 1, 1, event)
	ctrl.HandleEnvelope("d1", env)
	time.Sleep(20 * time.Millisecond)

	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != runstore.StatusQueued {
		t.Fatalf("superseded terminal event changed status to %q", current.Status)
	}
	publisher.mu.Lock()
	published := append([]string(nil), publisher.statuses...)
	publisher.mu.Unlock()
	if len(published) != 0 {
		t.Fatalf("published terminal state %v for a requeued run", published)
	}
	hub.mu.Lock()
	released := hub.releaseCalls
	hub.mu.Unlock()
	if released != 0 {
		t.Fatalf("released capacity %d times for a requeued run", released)
	}
}

func TestTerminalEventReleasesCapacity(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	hub := &fakeHub{}
	ctrl := New(runs, hub, Config{})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	data, _ := structpb.NewStruct(map[string]any{"status": "completed"})
	event := &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon, Data: data,
	}
	env, _ := protocol.NewRun("run-1", 1, 1, event)
	ctrl.HandleEnvelope("d1", env)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		released := len(hub.released)
		hub.mu.Unlock()
		if released == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("capacity was not released on terminal event")
}

func TestHandleNoDaemonRetries(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	ctrl := New(runs, &fakeHub{}, Config{RetryDelay: 3 * time.Second})
	err := ctrl.Handle(context.Background(), testTask(), workflow.Lease{})
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
	if _, ok := workflow.RetryAfterDuration(err); ok {
		t.Fatalf("unexpected explicit retry delay: %v", err)
	}
	if !strings.Contains(err.Error(), "no eligible daemon") {
		t.Fatalf("error = %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, ok := workflow.RetryAfterDuration(err); ok {
		t.Fatalf("unexpected explicit retry delay: %v", err)
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
	if !errors.Is(err, errAckTimeout) {
		t.Fatalf("Handle() error = %v, want errAckTimeout", err)
	}
	if _, ok := workflow.RetryAfterDuration(err); ok {
		t.Fatalf("unexpected explicit retry delay: %v", err)
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

func TestControlRecordsMetrics(t *testing.T) {
	runs := &fakeRuns{
		run:          newRun(),
		appendResult: runstore.AppendResult{Inserted: true},
	}
	runs.run.Status = "running"
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	registry := observe.NewRegistry()
	ctrl := New(runs, &fakeHub{}, Config{Metrics: registry})
	defer ctrl.Close()

	statusData, _ := structpb.NewStruct(map[string]any{"status": "completed"})
	statusEnv, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon, Data: statusData,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	ctrl.handleEvent(statusEnv)

	usageData, _ := structpb.NewStruct(map[string]any{"input_tokens": 10, "output_tokens": 5})
	usageEnv, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Event{
		Seq: 2, Type: runtimev1.EventType_USAGE, Source: protocol.EventSourceDaemon, Data: usageData,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	ctrl.handleEvent(usageEnv)

	var buf bytes.Buffer
	if err := registry.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		`mw_runs_total{runtime="normal",status="completed"} 1`,
		`mw_events_total{type="RUN_STATUS"} 1`,
		`mw_events_total{type="USAGE"} 1`,
		`mw_token_usage_total{app="app-1",model="claude-test"} 15`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("metrics do not contain %q:\n%s", want, output)
		}
	}
}

func TestRecoverQueuedRunsEnqueuesMissingTasks(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = runstore.StatusQueued
	tasks := &fakeTaskCanceller{}
	ctrl := New(runs, &fakeHub{}, Config{Tasks: tasks})

	if err := ctrl.RecoverQueued(context.Background()); err != nil {
		t.Fatalf("RecoverQueued() error = %v", err)
	}
	if tasks.enqueueCalls != 2 {
		t.Fatalf("enqueue calls = %d, want dispatch+timeout", tasks.enqueueCalls)
	}
}

func TestRecoverActiveRunsRequeuesDisconnectedDaemon(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = runstore.StatusAssigned
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	tasks := &fakeTaskCanceller{}
	hub := &fakeHub{}
	ctrl := New(runs, hub, Config{
		Tasks:           tasks,
		DaemonAvailable: func(string) bool { return false },
	})

	if err := ctrl.RecoverActive(context.Background()); err != nil {
		t.Fatalf("RecoverActive() error = %v", err)
	}
	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != runstore.StatusQueued {
		t.Fatalf("status = %q, want queued", current.Status)
	}
	if tasks.enqueueCalls != 1 || hub.releaseCalls != 1 {
		t.Fatalf("enqueue=%d release=%d", tasks.enqueueCalls, hub.releaseCalls)
	}
}

func TestRecoverActiveRunsLeavesConnectedDaemon(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	runs.run.Status = runstore.StatusRunning
	runs.run.Attempt = 1
	runs.run.LeaseEpoch = 1
	runs.run.AssignedDaemonID = "d1"
	tasks := &fakeTaskCanceller{}
	ctrl := New(runs, &fakeHub{}, Config{
		Tasks:           tasks,
		DaemonAvailable: func(string) bool { return true },
	})

	if err := ctrl.RecoverActive(context.Background()); err != nil {
		t.Fatalf("RecoverActive() error = %v", err)
	}
	current, _ := runs.GetRun(context.Background(), "run-1")
	if current.Status != runstore.StatusRunning || tasks.enqueueCalls != 0 {
		t.Fatalf("status=%s enqueue=%d", current.Status, tasks.enqueueCalls)
	}
}

type blockingInvoker struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingInvoker) Invoke(_ context.Context, _ string, _ string, request *runtimev1.ToolCallRequest) *runtimev1.ToolCallResult {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return &runtimev1.ToolCallResult{
		InvocationId: request.GetInvocationId(),
		Ok:           true,
		ResultJson:   `{"ok":true}`,
	}
}

func TestDispatchToolCallDoesNotBlockAndRejectsWhenFull(t *testing.T) {
	runs := &fakeRuns{run: newRun()}
	hub := &fakeHub{}
	invoker := &blockingInvoker{started: make(chan struct{}), release: make(chan struct{})}
	ctrl := New(runs, hub, Config{Tools: invoker, ToolConcurrency: 1})
	ctrl.Start(context.Background())
	defer ctrl.Close()

	env := func() *runtimev1.Envelope {
		request := &runtimev1.ToolCallRequest{
			InvocationId: id.New(), ToolName: "search", ArgumentsJson: "{}", TimeoutMs: 1000,
		}
		envelope, err := protocol.NewRun("run-1", 1, 1, request)
		if err != nil {
			t.Fatalf("NewRun() error = %v", err)
		}
		return envelope
	}

	first := env()
	dispatched := make(chan struct{})
	go func() {
		ctrl.DispatchToolCall("d1", first)
		close(dispatched)
	}()
	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatal("DispatchToolCall blocked while starting a tool")
	}
	select {
	case <-invoker.started:
	case <-time.After(time.Second):
		t.Fatal("first tool did not start")
	}

	second := env()
	secondDone := make(chan struct{})
	go func() {
		ctrl.DispatchToolCall("d1", second)
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("rejected DispatchToolCall blocked")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		sent := len(hub.sent)
		hub.mu.Unlock()
		if sent == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	hub.mu.Lock()
	busy := hub.sent[0].GetToolCallResult()
	hub.mu.Unlock()
	if busy == nil || busy.GetError().GetCode() != "TOOL_BUSY" {
		t.Fatalf("second call was not rejected with TOOL_BUSY: %+v", busy)
	}

	close(invoker.release)
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		sent := len(hub.sent)
		hub.mu.Unlock()
		if sent == 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("first tool result was not sent")
}
