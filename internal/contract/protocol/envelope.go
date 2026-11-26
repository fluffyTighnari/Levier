// Levier — internal/contract/protocol
//
// Envelope construction and correlation helpers.
//
// Copyright 2026 The Levier Authors.
package protocol

import (
	"fmt"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/platform/id"
)

// DefaultMaxEnvelopeBytes is the default gRPC envelope size limit.
const DefaultMaxEnvelopeBytes = 16 << 20

// Option customizes a new Envelope.
type Option func(*runtimev1.Envelope)

// WithMsgID overrides the generated message identifier.
func WithMsgID(msgID string) Option {
	return func(env *runtimev1.Envelope) {
		env.MsgId = msgID
	}
}

// WithCorrelationID sets the response correlation identifier.
func WithCorrelationID(correlationID string) Option {
	return func(env *runtimev1.Envelope) {
		env.CorrelationId = correlationID
	}
}

// WithRun sets run-scoped envelope fields.
func WithRun(runID string, attempt int32, leaseEpoch int64) Option {
	return func(env *runtimev1.Envelope) {
		env.RunId = runID
		env.Attempt = attempt
		env.LeaseEpoch = leaseEpoch
	}
}

// WithTimestamp overrides the envelope timestamp.
func WithTimestamp(tsMillis int64) Option {
	return func(env *runtimev1.Envelope) {
		env.TsMs = tsMillis
	}
}

// New builds and validates an envelope for payload.
func New(payload any, opts ...Option) (*runtimev1.Envelope, error) {
	env := &runtimev1.Envelope{
		MsgId: id.New(),
		TsMs:  time.Now().UTC().UnixMilli(),
	}
	wrapPayload(env, payload)

	for _, opt := range opts {
		if opt != nil {
			opt(env)
		}
	}

	if err := Validate(env); err != nil {
		return nil, err
	}
	return env, nil
}

// NewRun builds and validates a run-scoped envelope.
func NewRun(runID string, attempt int32, leaseEpoch int64, payload any, opts ...Option) (*runtimev1.Envelope, error) {
	options := make([]Option, 0, len(opts)+1)
	options = append(options, WithRun(runID, attempt, leaseEpoch))
	options = append(options, opts...)
	return New(payload, options...)
}

// NewResponse builds a response correlated to request.
func NewResponse(request *runtimev1.Envelope, payload any, opts ...Option) (*runtimev1.Envelope, error) {
	if request == nil {
		return nil, fmt.Errorf("request envelope is nil")
	}
	if request.GetMsgId() == "" {
		return nil, fmt.Errorf("request msg_id is empty")
	}

	options := make([]Option, 0, len(opts)+2)
	options = append(options,
		WithRun(request.GetRunId(), request.GetAttempt(), request.GetLeaseEpoch()),
		WithCorrelationID(request.GetMsgId()),
	)
	options = append(options, opts...)
	return New(payload, options...)
}

// SetCorrelation sets response.correlation_id from request.msg_id.
func SetCorrelation(response, request *runtimev1.Envelope) error {
	if response == nil || request == nil {
		return fmt.Errorf("response and request envelopes are required")
	}
	if request.GetMsgId() == "" {
		return fmt.Errorf("request msg_id is empty")
	}
	response.CorrelationId = request.GetMsgId()
	return nil
}

// PayloadName returns a stable label for logging and metrics.
func PayloadName(env *runtimev1.Envelope) string {
	if env == nil {
		return "nil"
	}
	switch env.GetPayload().(type) {
	case *runtimev1.Envelope_Hello:
		return "hello"
	case *runtimev1.Envelope_Heartbeat:
		return "heartbeat"
	case *runtimev1.Envelope_TaskAssign:
		return "task_assign"
	case *runtimev1.Envelope_TaskAck:
		return "task_ack"
	case *runtimev1.Envelope_Event:
		return "event"
	case *runtimev1.Envelope_ToolCallRequest:
		return "tool_call_request"
	case *runtimev1.Envelope_ToolCallResult:
		return "tool_call_result"
	case *runtimev1.Envelope_Cancel:
		return "cancel"
	case *runtimev1.Envelope_Reconcile:
		return "reconcile"
	case *runtimev1.Envelope_Checkpoint:
		return "checkpoint"
	case *runtimev1.Envelope_Error:
		return "error"
	default:
		return "unknown"
	}
}

func wrapPayload(env *runtimev1.Envelope, payload any) {
	switch typed := payload.(type) {
	case *runtimev1.Hello:
		env.Payload = &runtimev1.Envelope_Hello{Hello: typed}
	case *runtimev1.Heartbeat:
		env.Payload = &runtimev1.Envelope_Heartbeat{Heartbeat: typed}
	case *runtimev1.TaskAssign:
		env.Payload = &runtimev1.Envelope_TaskAssign{TaskAssign: typed}
	case *runtimev1.TaskAck:
		env.Payload = &runtimev1.Envelope_TaskAck{TaskAck: typed}
	case *runtimev1.Event:
		env.Payload = &runtimev1.Envelope_Event{Event: typed}
	case *runtimev1.ToolCallRequest:
		env.Payload = &runtimev1.Envelope_ToolCallRequest{ToolCallRequest: typed}
	case *runtimev1.ToolCallResult:
		env.Payload = &runtimev1.Envelope_ToolCallResult{ToolCallResult: typed}
	case *runtimev1.Cancel:
		env.Payload = &runtimev1.Envelope_Cancel{Cancel: typed}
	case *runtimev1.Reconcile:
		env.Payload = &runtimev1.Envelope_Reconcile{Reconcile: typed}
	case *runtimev1.Checkpoint:
		env.Payload = &runtimev1.Envelope_Checkpoint{Checkpoint: typed}
	case *runtimev1.ErrorReport:
		env.Payload = &runtimev1.Envelope_Error{Error: typed}
	}
}
