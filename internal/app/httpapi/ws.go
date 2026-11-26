// Levier — internal/app/httpapi
//
// App WebSocket contract types and client message parsing.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/fluffyTighnari/levier/internal/contract"
)

// ClientMessage is any App→middleware WebSocket message.
type ClientMessage interface {
	isClientMessage()
}

// ServerMessage is any middleware→App WebSocket message.
type ServerMessage interface {
	isServerMessage()
}

// HelloMessage is the optional v1/v2 handshake.
type HelloMessage struct {
	Type            string        `json:"type"`
	Protocol        int           `json:"protocol"`
	ProtocolVersion int           `json:"protocol_version,omitempty"`
	Client          string        `json:"client,omitempty"`
	Version         string        `json:"version,omitempty"`
	ConnectionID    string        `json:"connection_id,omitempty"`
	AppID           string        `json:"app_id,omitempty"`
	Capabilities    []string      `json:"capabilities,omitempty"`
	ResumeCursor    *ResumeCursor `json:"resume_cursor,omitempty"`
}

// ResumeCursor is the last event position the App has processed for one Run.
type ResumeCursor struct {
	RunID   string `json:"run_id"`
	Attempt int32  `json:"last_attempt"`
	Seq     int64  `json:"last_seq"`
}

// NegotiatedVersion returns the protocol version requested by the client.
// v2 clients may use either protocol_version or the legacy protocol field.
func (m HelloMessage) NegotiatedVersion() int {
	if m.ProtocolVersion != 0 {
		return m.ProtocolVersion
	}
	return m.Protocol
}

// HelloAckMessage is the v2 handshake response.
type HelloAckMessage struct {
	Type                string   `json:"type"`
	Protocol            int      `json:"protocol"`
	ConnectionID        string   `json:"connection_id"`
	AppID               string   `json:"app_id"`
	Capabilities        []string `json:"capabilities"`
	HeartbeatIntervalMS int64    `json:"heartbeat_interval_ms"`
	ServerVersion       string   `json:"server_version,omitempty"`
}

// SubscribeMessage subscribes to a run stream.
type SubscribeMessage struct {
	Type        string `json:"type"`
	RunID       string `json:"run_id"`
	LastAttempt int32  `json:"last_attempt"`
	LastSeq     int64  `json:"last_seq"`
}

// LocalToolResultMessage returns a local tool result.
type LocalToolResultMessage struct {
	Type          string     `json:"type"`
	CorrelationID string     `json:"correlation_id"`
	InvocationID  string     `json:"invocation_id"`
	RunID         string     `json:"run_id,omitempty"`
	OK            bool       `json:"ok"`
	ResultJSON    string     `json:"result_json,omitempty"`
	Error         *ToolError `json:"error,omitempty"`
	Idempotent    bool       `json:"idempotent,omitempty"`
}

// LocalToolProgressMessage reports progress for a running local tool call.
type LocalToolProgressMessage struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	InvocationID  string          `json:"invocation_id"`
	RunID         string          `json:"run_id,omitempty"`
	Progress      json.RawMessage `json:"progress_json"`
}

// CancelMessage asks the App to cancel one local tool call.
type CancelMessage struct {
	Type         string `json:"type"`
	InvocationID string `json:"invocation_id"`
	Reason       string `json:"reason,omitempty"`
}

// PingMessage is a client keepalive.
type PingMessage struct {
	Type string `json:"type"`
}

// EventMessage is a server run event.
type EventMessage struct {
	Type          string          `json:"type"`
	RunID         string          `json:"run_id"`
	Attempt       int32           `json:"attempt"`
	Seq           int64           `json:"seq"`
	Source        string          `json:"source"`
	EventType     string          `json:"event_type"`
	EventName     string          `json:"event_name,omitempty"`
	SchemaVersion string          `json:"schema_version,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	TSMillis      int64           `json:"ts_ms"`
}

// LocalToolCallMessage asks the App to execute a local tool.
type LocalToolCallMessage struct {
	Type          string `json:"type"`
	MsgID         string `json:"msg_id"`
	InvocationID  string `json:"invocation_id"`
	RunID         string `json:"run_id,omitempty"`
	ToolName      string `json:"tool_name"`
	ArgumentsJSON string `json:"arguments_json"`
	TimeoutMS     int64  `json:"timeout_ms"`
	DeadlineMS    int64  `json:"deadline_ms,omitempty"`
	Idempotent    bool   `json:"idempotent,omitempty"`
}

// RunCompletedMessage announces a terminal run.
type RunCompletedMessage struct {
	Type    string    `json:"type"`
	RunID   string    `json:"run_id"`
	Status  RunStatus `json:"status"`
	Attempt int32     `json:"attempt"`
}

// PongMessage is a server keepalive.
type PongMessage struct {
	Type string `json:"type"`
}

// CaughtUpMessage marks replay completion.
type CaughtUpMessage struct {
	Type    string `json:"type"`
	RunID   string `json:"run_id"`
	Attempt int32  `json:"attempt"`
	Seq     int64  `json:"seq"`
}

// ErrorMessage is a server-side WebSocket error.
type ErrorMessage struct {
	Type  string    `json:"type"`
	Error ErrorBody `json:"error"`
}

// ToolError is a local tool execution error.
type ToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (*HelloMessage) isClientMessage()             {}
func (*SubscribeMessage) isClientMessage()         {}
func (*LocalToolResultMessage) isClientMessage()   {}
func (*LocalToolProgressMessage) isClientMessage() {}
func (*PingMessage) isClientMessage()              {}

func (*HelloAckMessage) isServerMessage()      {}
func (*EventMessage) isServerMessage()         {}
func (*LocalToolCallMessage) isServerMessage() {}
func (*CancelMessage) isServerMessage()        {}
func (*RunCompletedMessage) isServerMessage()  {}
func (*PongMessage) isServerMessage()          {}
func (*CaughtUpMessage) isServerMessage()      {}
func (*ErrorMessage) isServerMessage()         {}

// ParseClientMessage decodes and validates one App→middleware message.
func ParseClientMessage(data []byte) (ClientMessage, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode ws message: %w", err)
	}

	switch probe.Type {
	case contract.WSMessageHello:
		var msg HelloMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("decode hello: %w", err)
		}
		if err := msg.Validate(); err != nil {
			return nil, err
		}
		return &msg, nil
	case contract.WSMessageSubscribe:
		var msg SubscribeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("decode subscribe: %w", err)
		}
		if err := msg.Validate(); err != nil {
			return nil, err
		}
		return &msg, nil
	case contract.WSMessageLocalToolResult:
		var msg LocalToolResultMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("decode local_tool_result: %w", err)
		}
		if err := msg.Validate(); err != nil {
			return nil, err
		}
		return &msg, nil
	case contract.WSMessageLocalToolProgress:
		var msg LocalToolProgressMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("decode local_tool_progress: %w", err)
		}
		if err := msg.Validate(); err != nil {
			return nil, err
		}
		return &msg, nil
	case contract.WSMessagePing:
		var msg PingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("decode ping: %w", err)
		}
		return &msg, nil
	default:
		return nil, fmt.Errorf("unsupported ws message type %q", probe.Type)
	}
}

// Validate checks the optional v1/v2 handshake.
func (m HelloMessage) Validate() error {
	if m.Type != contract.WSMessageHello {
		return fmt.Errorf("hello.type must be %q", contract.WSMessageHello)
	}
	if m.Protocol != 0 && m.ProtocolVersion != 0 && m.Protocol != m.ProtocolVersion {
		return fmt.Errorf("hello protocol and protocol_version disagree")
	}
	switch m.NegotiatedVersion() {
	case contract.WSProtocolVersion, contract.WSProtocolVersionV2:
	default:
		return fmt.Errorf("unsupported ws protocol %d", m.NegotiatedVersion())
	}
	if m.ResumeCursor != nil {
		if m.ResumeCursor.RunID == "" {
			return fmt.Errorf("hello.resume_cursor.run_id is required")
		}
		if m.ResumeCursor.Attempt < 0 || m.ResumeCursor.Seq < 0 {
			return fmt.Errorf("hello.resume_cursor cursors must not be negative")
		}
	}
	return nil
}

// Validate checks a subscribe message.
func (m SubscribeMessage) Validate() error {
	if m.Type != contract.WSMessageSubscribe {
		return fmt.Errorf("subscribe.type must be %q", contract.WSMessageSubscribe)
	}
	if m.RunID == "" {
		return fmt.Errorf("subscribe.run_id is required")
	}
	if m.LastAttempt < 0 || m.LastSeq < 0 {
		return fmt.Errorf("subscribe cursors must not be negative")
	}
	return nil
}

// Validate checks a local tool result.
func (m LocalToolResultMessage) Validate() error {
	if m.Type != contract.WSMessageLocalToolResult {
		return fmt.Errorf("local_tool_result.type must be %q", contract.WSMessageLocalToolResult)
	}
	if m.CorrelationID == "" {
		return fmt.Errorf("local_tool_result.correlation_id is required")
	}
	if m.InvocationID == "" {
		return fmt.Errorf("local_tool_result.invocation_id is required")
	}
	if m.OK && m.Error != nil {
		return fmt.Errorf("local_tool_result.error must be nil when ok")
	}
	if !m.OK && m.Error == nil {
		return fmt.Errorf("local_tool_result.error is required when not ok")
	}
	return nil
}

// Validate checks a local tool progress message.
func (m LocalToolProgressMessage) Validate() error {
	if m.Type != contract.WSMessageLocalToolProgress {
		return fmt.Errorf("local_tool_progress.type must be %q", contract.WSMessageLocalToolProgress)
	}
	if m.InvocationID == "" {
		return fmt.Errorf("local_tool_progress.invocation_id is required")
	}
	if len(m.Progress) == 0 {
		return fmt.Errorf("local_tool_progress.progress_json is required")
	}
	return nil
}
