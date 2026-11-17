// Levier — internal/protocol
//
// Envelope construction, correlation, and validation tests.
//
// Copyright 2026 The Levier Authors.
package protocol

import (
	"strings"
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/errs"
	"github.com/fluffyTighnari/levier/internal/id"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func mustNew(t *testing.T, payload any, opts ...Option) *runtimev1.Envelope {
	t.Helper()
	env, err := New(payload, opts...)
	if err != nil {
		t.Fatalf("New(%T) error = %v", payload, err)
	}
	return env
}

func validHello() *runtimev1.Hello {
	return &runtimev1.Hello{
		DaemonId:            "daemon-1",
		AuthToken:           "token",
		RuntimeType:         runtimev1.RuntimeType_NORMAL,
		MaxConcurrentRuns:   4,
		SupportedStrategies: []string{"react"},
		Version:             "0.1.0",
		RunningRuns: []*runtimev1.RunningRun{
			{RunId: "run-1", LeaseEpoch: 1},
		},
	}
}

func validTaskAssign(runID string) *runtimev1.TaskAssign {
	return &runtimev1.TaskAssign{
		RunId:    runID,
		AppId:    "app-1",
		Strategy: "react",
		Input: []*runtimev1.ChatMessage{
			{Role: "user", Content: "hello"},
		},
		Model: &runtimev1.ModelProfile{
			Provider: "openai",
			Model:    "gpt-test",
		},
		Tools: []*runtimev1.ToolManifest{
			{Name: "echo", InputSchemaJson: "{}"},
		},
		McpBindings: []*runtimev1.McpBinding{
			{ServerName: "fs", Transport: "stdio", Command: "mcp-fs", AllowedTools: []string{"read"}},
		},
		Timeouts: &runtimev1.TimeoutProfile{
			RunTimeoutMs:  1800000,
			ToolTimeoutMs: 60000,
		},
	}
}

func validEvent() *runtimev1.Event {
	return &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: "daemon",
	}
}

func TestNewHelloAndPayloadName(t *testing.T) {
	env := mustNew(t, validHello())
	if err := Validate(env); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := PayloadName(env); got != "hello" {
		t.Fatalf("PayloadName() = %q", got)
	}
	if env.GetRunId() != "" {
		t.Fatalf("hello run_id = %q, want empty", env.GetRunId())
	}
}

func TestNewRunTaskAssignAndAck(t *testing.T) {
	runID := "run-1"
	assign := validTaskAssign(runID)
	req, err := NewRun(runID, 1, 1, assign)
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if PayloadName(req) != "task_assign" {
		t.Fatalf("PayloadName() = %q", PayloadName(req))
	}

	ack, err := NewResponse(req, &runtimev1.TaskAck{Accepted: true})
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}
	if ack.GetCorrelationId() != req.GetMsgId() {
		t.Fatalf("correlation_id = %q, want %q", ack.GetCorrelationId(), req.GetMsgId())
	}
	if ack.GetRunId() != runID || ack.GetAttempt() != 1 || ack.GetLeaseEpoch() != 1 {
		t.Fatalf("response scope = (%q,%d,%d)", ack.GetRunId(), ack.GetAttempt(), ack.GetLeaseEpoch())
	}
}

func TestEventSeqRules(t *testing.T) {
	persistent := mustNew(t, validEvent(), WithRun("run-1", 1, 1))
	if persistent.GetCorrelationId() != "" {
		t.Fatal("event should not have correlation_id")
	}

	ephemeral := mustNew(t, &runtimev1.Event{
		Seq:       0,
		Type:      runtimev1.EventType_MESSAGE_DELTA,
		Ephemeral: true,
		Source:    "daemon",
	}, WithRun("run-1", 1, 1))
	if ephemeral.GetEvent().GetSeq() != 0 {
		t.Fatal("ephemeral seq should be zero")
	}

	mw := mustNew(t, &runtimev1.Event{
		Seq:    MWSeqBase,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: "mw",
	}, WithRun("run-1", 1, 1))
	if mw.GetEvent().GetSeq() != MWSeqBase {
		t.Fatal("mw seq mismatch")
	}
}

func TestToolCallRoundTrip(t *testing.T) {
	invocationID := id.New()
	req, err := NewRun("run-1", 1, 1, &runtimev1.ToolCallRequest{
		InvocationId:  invocationID,
		ToolName:      "echo",
		ArgumentsJson: "{}",
		TimeoutMs:     60000,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}

	result, err := NewResponse(req, &runtimev1.ToolCallResult{
		InvocationId: invocationID,
		Ok:           true,
		ResultJson:   `{"ok":true}`,
	})
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}
	if result.GetToolCallResult().GetInvocationId() != invocationID {
		t.Fatal("invocation id mismatch")
	}
}

func TestValidateRejectsInvalidEnvelopes(t *testing.T) {
	base := mustNew(t, validHello())
	assign := mustNew(t, validTaskAssign("run-1"), WithRun("run-1", 1, 1))
	event := mustNew(t, validEvent(), WithRun("run-1", 1, 1))
	ack := mustNew(t, &runtimev1.TaskAck{Accepted: true}, WithRun("run-1", 1, 1), WithCorrelationID(id.New()))
	checkpoint := mustNew(t, &runtimev1.Checkpoint{SessionId: "s-1"}, WithRun("run-1", 1, 1))

	tests := []struct {
		name string
		env  *runtimev1.Envelope
		want string
	}{
		{"nil", nil, "envelope is nil"},
		{"bad msg id", &runtimev1.Envelope{}, "msg_id"},
		{"no payload", &runtimev1.Envelope{MsgId: id.New(), TsMs: 1}, "payload is required"},
		{"hello with run", withRun(base, "run-1"), "hello must not carry run_id"},
		{"assign missing app", mutateAssign(assign, func(a *runtimev1.TaskAssign) { a.AppId = "" }), "app_id"},
		{"assign duplicate tool", mutateAssign(assign, func(a *runtimev1.TaskAssign) {
			a.Tools = append(a.Tools, &runtimev1.ToolManifest{Name: "echo", InputSchemaJson: "{}"})
		}), "duplicate name"},
		{"event ephemeral seq", mutateEvent(event, func(e *runtimev1.Event) { e.Ephemeral = true }), "ephemeral event seq"},
		{"event mw low seq", mutateEvent(event, func(e *runtimev1.Event) { e.Source = "mw" }), "mw event seq"},
		{"ack missing correlation", mutateAck(ack, func(e *runtimev1.Envelope) { e.CorrelationId = "" }), "correlation_id"},
		{"checkpoint missing session", mutateCheckpoint(checkpoint), "session_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsBadMcpBinding(t *testing.T) {
	assign := validTaskAssign("run-1")
	assign.McpBindings[0].Command = ""
	_, err := NewRun("run-1", 1, 1, assign)
	if err == nil || !strings.Contains(err.Error(), "requires command") {
		t.Fatalf("NewRun() error = %v", err)
	}
}

func TestCheckSize(t *testing.T) {
	large, err := structpb.NewStruct(map[string]any{"payload": strings.Repeat("x", 2048)})
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	env := mustNew(t, &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_MESSAGE_COMPLETED,
		Source: "daemon",
		Data:   large,
	}, WithRun("run-1", 1, 1))

	if err := CheckSize(env, 64); err == nil {
		t.Fatal("CheckSize() error = nil, want size error")
	}
}

func TestSetCorrelation(t *testing.T) {
	req := mustNew(t, validHello())
	env := mustNew(t, &runtimev1.Heartbeat{})
	if err := SetCorrelation(env, req); err != nil {
		t.Fatalf("SetCorrelation() error = %v", err)
	}
	if env.GetCorrelationId() != req.GetMsgId() {
		t.Fatal("correlation mismatch")
	}
}

func TestValidateReturnsBadEnvelopeCode(t *testing.T) {
	err := Validate(&runtimev1.Envelope{})
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if got := errs.CodeOf(err); got != errs.CodeBadEnvelope {
		t.Fatalf("CodeOf(Validate()) = %q, want BAD_ENVELOPE", got)
	}
}

func TestNewRejectsUnknownPayload(t *testing.T) {
	if _, err := New("not-a-payload"); err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func withRun(env *runtimev1.Envelope, runID string) *runtimev1.Envelope {
	clone := cloneEnvelope(env)
	clone.RunId = runID
	return clone
}

func mutateAssign(env *runtimev1.Envelope, mutate func(*runtimev1.TaskAssign)) *runtimev1.Envelope {
	clone := cloneEnvelope(env)
	mutate(clone.GetTaskAssign())
	return clone
}

func mutateEvent(env *runtimev1.Envelope, mutate func(*runtimev1.Event)) *runtimev1.Envelope {
	clone := cloneEnvelope(env)
	mutate(clone.GetEvent())
	return clone
}

func mutateAck(env *runtimev1.Envelope, mutate func(*runtimev1.Envelope)) *runtimev1.Envelope {
	clone := cloneEnvelope(env)
	mutate(clone)
	return clone
}

func mutateCheckpoint(env *runtimev1.Envelope) *runtimev1.Envelope {
	clone := cloneEnvelope(env)
	clone.GetCheckpoint().SessionId = ""
	return clone
}

func cloneEnvelope(env *runtimev1.Envelope) *runtimev1.Envelope {
	return proto.Clone(env).(*runtimev1.Envelope)
}
