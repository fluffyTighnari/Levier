// Levier — internal/contract/protocol
//
// Envelope validation rules.
//
// Copyright 2026 The Levier Authors.
package protocol

import (
	"fmt"
	"strings"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/platform/errs"
	"github.com/fluffyTighnari/levier/internal/platform/id"
	"google.golang.org/protobuf/proto"
)

// MWSeqBase is the first sequence number reserved for middleware events.
const MWSeqBase int64 = 1 << 62

const (
	// EventSourceDaemon identifies events produced by a harness.
	EventSourceDaemon = "daemon"
	// EventSourceMW identifies events synthesized by middleware.
	EventSourceMW = "mw"
)

// Validate checks envelope shape, identifiers, run scope, and payload fields.
// Validation failures are returned as BAD_ENVELOPE errors.
func Validate(env *runtimev1.Envelope) error {
	if err := validate(env); err != nil {
		return errs.Wrap(errs.CodeBadEnvelope, "invalid envelope", err)
	}
	return nil
}

func validate(env *runtimev1.Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	if _, err := id.Parse(env.GetMsgId()); err != nil {
		return fmt.Errorf("msg_id: %w", err)
	}
	if env.GetCorrelationId() != "" {
		if _, err := id.Parse(env.GetCorrelationId()); err != nil {
			return fmt.Errorf("correlation_id: %w", err)
		}
	}
	if env.GetTsMs() <= 0 {
		return fmt.Errorf("ts_ms must be positive")
	}
	if env.GetAttempt() < 0 {
		return fmt.Errorf("attempt must not be negative")
	}
	if env.GetLeaseEpoch() < 0 {
		return fmt.Errorf("lease_epoch must not be negative")
	}
	if env.GetPayload() == nil {
		return fmt.Errorf("payload is required")
	}

	if err := CheckSize(env, DefaultMaxEnvelopeBytes); err != nil {
		return err
	}

	switch payload := env.GetPayload().(type) {
	case *runtimev1.Envelope_Hello:
		return validateHello(env, payload.Hello)
	case *runtimev1.Envelope_Heartbeat:
		return validateHeartbeat(env, payload.Heartbeat)
	case *runtimev1.Envelope_TaskAssign:
		return validateTaskAssign(env, payload.TaskAssign)
	case *runtimev1.Envelope_TaskAck:
		return validateTaskAck(env, payload.TaskAck)
	case *runtimev1.Envelope_Event:
		return validateEvent(env, payload.Event)
	case *runtimev1.Envelope_ToolCallRequest:
		return validateToolCallRequest(env, payload.ToolCallRequest)
	case *runtimev1.Envelope_ToolCallResult:
		return validateToolCallResult(env, payload.ToolCallResult)
	case *runtimev1.Envelope_Cancel:
		return validateCancel(env, payload.Cancel)
	case *runtimev1.Envelope_Reconcile:
		return validateReconcile(env, payload.Reconcile)
	case *runtimev1.Envelope_Checkpoint:
		return validateCheckpoint(env, payload.Checkpoint)
	case *runtimev1.Envelope_Error:
		return validateError(payload.Error)
	default:
		return fmt.Errorf("unsupported payload %T", env.GetPayload())
	}
}

// CheckSize rejects envelopes larger than maxBytes.
func CheckSize(env *runtimev1.Envelope, maxBytes int) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxEnvelopeBytes
	}
	if size := proto.Size(env); size > maxBytes {
		return fmt.Errorf("envelope size %d exceeds limit %d", size, maxBytes)
	}
	return nil
}

func validateHello(env *runtimev1.Envelope, hello *runtimev1.Hello) error {
	if env.GetRunId() != "" {
		return fmt.Errorf("hello must not carry run_id")
	}
	if hello == nil {
		return fmt.Errorf("hello is nil")
	}
	if hello.GetDaemonId() == "" {
		return fmt.Errorf("hello.daemon_id is required")
	}
	if hello.GetAuthToken() == "" {
		return fmt.Errorf("hello.auth_token is required")
	}
	if hello.GetRuntimeType() == runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED {
		return fmt.Errorf("hello.runtime_type is required")
	}
	if hello.GetRuntimeName() == "" {
		return fmt.Errorf("hello.runtime_name is required")
	}
	if hello.GetMaxConcurrentRuns() <= 0 {
		return fmt.Errorf("hello.max_concurrent_runs must be positive")
	}
	seen := make(map[string]struct{}, len(hello.GetRunningRuns()))
	for _, running := range hello.GetRunningRuns() {
		if running.GetRunId() == "" {
			return fmt.Errorf("hello.running_runs contains empty run_id")
		}
		if running.GetLeaseEpoch() < 0 {
			return fmt.Errorf("hello.running_runs[%s].lease_epoch must not be negative", running.GetRunId())
		}
		if _, ok := seen[running.GetRunId()]; ok {
			return fmt.Errorf("hello.running_runs contains duplicate run_id %q", running.GetRunId())
		}
		seen[running.GetRunId()] = struct{}{}
	}
	return nil
}

func validateHeartbeat(env *runtimev1.Envelope, heartbeat *runtimev1.Heartbeat) error {
	if env.GetRunId() != "" {
		return fmt.Errorf("heartbeat must not carry run_id")
	}
	if heartbeat == nil {
		return fmt.Errorf("heartbeat is nil")
	}
	if heartbeat.GetRunningCount() < 0 {
		return fmt.Errorf("heartbeat.running_count must not be negative")
	}
	return nil
}

func validateTaskAssign(env *runtimev1.Envelope, assign *runtimev1.TaskAssign) error {
	if assign == nil {
		return fmt.Errorf("task_assign is nil")
	}
	if err := requireRunScope(env, "task_assign"); err != nil {
		return err
	}
	if env.GetAttempt() < 1 {
		return fmt.Errorf("task_assign requires attempt >= 1")
	}
	if env.GetLeaseEpoch() < 1 {
		return fmt.Errorf("task_assign requires lease_epoch >= 1")
	}
	if assign.GetRunId() != env.GetRunId() {
		return fmt.Errorf("task_assign.run_id must match envelope run_id")
	}
	if assign.GetAppId() == "" {
		return fmt.Errorf("task_assign.app_id is required")
	}
	if assign.GetModel() != nil &&
		(assign.GetModel().GetProvider() == "" || assign.GetModel().GetModel() == "") {
		return fmt.Errorf("task_assign.model.provider and model must both be set")
	}
	if assign.GetTimeouts() == nil || assign.GetTimeouts().GetRunTimeoutMs() <= 0 || assign.GetTimeouts().GetToolTimeoutMs() <= 0 {
		return fmt.Errorf("task_assign.timeouts must be positive")
	}
	toolNames := make(map[string]struct{}, len(assign.GetTools()))
	for _, tool := range assign.GetTools() {
		if tool.GetName() == "" {
			return fmt.Errorf("task_assign.tools contains empty name")
		}
		if tool.GetInputSchemaJson() == "" {
			return fmt.Errorf("task_assign.tools[%s].input_schema_json is required", tool.GetName())
		}
		if _, ok := toolNames[tool.GetName()]; ok {
			return fmt.Errorf("task_assign.tools contains duplicate name %q", tool.GetName())
		}
		toolNames[tool.GetName()] = struct{}{}
	}
	mcpNames := make(map[string]struct{}, len(assign.GetMcpBindings()))
	for _, binding := range assign.GetMcpBindings() {
		if err := validateMcpBinding(binding); err != nil {
			return err
		}
		if _, ok := mcpNames[binding.GetServerName()]; ok {
			return fmt.Errorf("task_assign.mcp_bindings contains duplicate server %q", binding.GetServerName())
		}
		mcpNames[binding.GetServerName()] = struct{}{}
	}
	return nil
}

func validateTaskAck(env *runtimev1.Envelope, ack *runtimev1.TaskAck) error {
	if ack == nil {
		return fmt.Errorf("task_ack is nil")
	}
	if err := requireRunScope(env, "task_ack"); err != nil {
		return err
	}
	if env.GetCorrelationId() == "" {
		return fmt.Errorf("task_ack requires correlation_id")
	}
	if !ack.GetAccepted() && ack.GetReason() == "" {
		return fmt.Errorf("task_ack.reason is required when rejected")
	}
	return nil
}

func validateEvent(env *runtimev1.Envelope, event *runtimev1.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if err := requireRunScope(env, "event"); err != nil {
		return err
	}
	if event.GetType() == runtimev1.EventType_EVENT_TYPE_UNSPECIFIED {
		return fmt.Errorf("event.type is required")
	}
	if event.GetType() == runtimev1.EventType_CUSTOM {
		if event.GetName() == "" {
			return fmt.Errorf("event.name is required for CUSTOM events")
		}
	} else if event.GetName() != "" || event.GetSchemaVersion() != "" {
		return fmt.Errorf("event.name and event.schema_version are only valid for CUSTOM events")
	}
	if event.GetSource() != EventSourceDaemon && event.GetSource() != EventSourceMW {
		return fmt.Errorf("event.source must be daemon or mw")
	}
	if event.GetEphemeral() {
		if event.GetSeq() != 0 {
			return fmt.Errorf("ephemeral event seq must be 0")
		}
		if event.GetSource() != EventSourceDaemon {
			return fmt.Errorf("ephemeral event source must be daemon")
		}
		return nil
	}
	if event.GetSeq() <= 0 {
		return fmt.Errorf("persistent event seq must be positive")
	}
	if event.GetSource() == EventSourceMW && event.GetSeq() < MWSeqBase {
		return fmt.Errorf("mw event seq must be >= %d", MWSeqBase)
	}
	if event.GetSource() == EventSourceDaemon && event.GetSeq() >= MWSeqBase {
		return fmt.Errorf("daemon event seq must be < %d", MWSeqBase)
	}
	return nil
}

func validateToolCallRequest(env *runtimev1.Envelope, req *runtimev1.ToolCallRequest) error {
	if req == nil {
		return fmt.Errorf("tool_call_request is nil")
	}
	if err := requireRunScope(env, "tool_call_request"); err != nil {
		return err
	}
	if _, err := id.Parse(req.GetInvocationId()); err != nil {
		return fmt.Errorf("tool_call_request.invocation_id: %w", err)
	}
	if req.GetToolName() == "" {
		return fmt.Errorf("tool_call_request.tool_name is required")
	}
	if req.GetTimeoutMs() <= 0 {
		return fmt.Errorf("tool_call_request.timeout_ms must be positive")
	}
	return nil
}

func validateToolCallResult(env *runtimev1.Envelope, result *runtimev1.ToolCallResult) error {
	if result == nil {
		return fmt.Errorf("tool_call_result is nil")
	}
	if err := requireRunScope(env, "tool_call_result"); err != nil {
		return err
	}
	if env.GetCorrelationId() == "" {
		return fmt.Errorf("tool_call_result requires correlation_id")
	}
	if _, err := id.Parse(result.GetInvocationId()); err != nil {
		return fmt.Errorf("tool_call_result.invocation_id: %w", err)
	}
	if result.GetOk() && result.GetError() != nil {
		return fmt.Errorf("tool_call_result.error must be nil when ok")
	}
	if !result.GetOk() && result.GetError() == nil {
		return fmt.Errorf("tool_call_result.error is required when not ok")
	}
	return nil
}

func validateCancel(env *runtimev1.Envelope, cancel *runtimev1.Cancel) error {
	if cancel == nil {
		return fmt.Errorf("cancel is nil")
	}
	if err := requireRunScope(env, "cancel"); err != nil {
		return err
	}
	if cancel.GetReason() == "" {
		return fmt.Errorf("cancel.reason is required")
	}
	return nil
}

func validateReconcile(env *runtimev1.Envelope, reconcile *runtimev1.Reconcile) error {
	if reconcile == nil {
		return fmt.Errorf("reconcile is nil")
	}
	if env.GetRunId() != "" {
		return fmt.Errorf("reconcile must not carry run_id")
	}
	seen := make(map[string]struct{}, len(reconcile.GetCancelRunIds()))
	for _, runID := range reconcile.GetCancelRunIds() {
		if runID == "" {
			return fmt.Errorf("reconcile.cancel_run_ids contains empty value")
		}
		if _, ok := seen[runID]; ok {
			return fmt.Errorf("reconcile.cancel_run_ids contains duplicate %q", runID)
		}
		seen[runID] = struct{}{}
	}
	return nil
}

func validateCheckpoint(env *runtimev1.Envelope, checkpoint *runtimev1.Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if err := requireRunScope(env, "checkpoint"); err != nil {
		return err
	}
	if checkpoint.GetSessionId() == "" {
		return fmt.Errorf("checkpoint.session_id is required")
	}
	if checkpoint.GetEventSeqWatermark() < 0 {
		return fmt.Errorf("checkpoint.event_seq_watermark must not be negative")
	}
	return nil
}

func validateError(report *runtimev1.ErrorReport) error {
	if report == nil {
		return fmt.Errorf("error is nil")
	}
	if report.GetCode() == "" {
		return fmt.Errorf("error.code is required")
	}
	if report.GetMessage() == "" {
		return fmt.Errorf("error.message is required")
	}
	return nil
}

func validateMcpBinding(binding *runtimev1.McpBinding) error {
	if binding == nil {
		return fmt.Errorf("mcp binding is nil")
	}
	if binding.GetServerName() == "" {
		return fmt.Errorf("mcp binding.server_name is required")
	}
	switch binding.GetTransport() {
	case "stdio":
		if binding.GetCommand() == "" {
			return fmt.Errorf("mcp binding %q requires command", binding.GetServerName())
		}
	case "sse", "streamable_http":
		if binding.GetUrl() == "" {
			return fmt.Errorf("mcp binding %q requires url", binding.GetServerName())
		}
	default:
		return fmt.Errorf("mcp binding %q has unsupported transport %q", binding.GetServerName(), binding.GetTransport())
	}
	for _, tool := range binding.GetAllowedTools() {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("mcp binding %q has empty allowed tool", binding.GetServerName())
		}
	}
	return nil
}

func requireRunScope(env *runtimev1.Envelope, payload string) error {
	if env.GetRunId() == "" {
		return fmt.Errorf("%s requires run_id", payload)
	}
	if env.GetAttempt() < 1 {
		return fmt.Errorf("%s requires attempt >= 1", payload)
	}
	if env.GetLeaseEpoch() < 1 {
		return fmt.Errorf("%s requires lease_epoch >= 1", payload)
	}
	return nil
}
