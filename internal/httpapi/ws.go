// Middleware — internal/httpapi
//
// App WebSocket contract types and client message parsing.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/contract"
)

// ClientMessage is any App→middleware WebSocket message.
type ClientMessage interface {
	isClientMessage()
}

// ServerMessage is any middleware→App WebSocket message.
type ServerMessage interface {
	isServerMessage()
}

// HelloMessage is the optional v1 handshake.
type HelloMessage struct {
	Type     string `json:"type"`
	Protocol int    `json:"protocol"`
	Client   string `json:"client,omitempty"`
	Version  string `json:"version,omitempty"`
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
	OK            bool       `json:"ok"`
	ResultJSON    string     `json:"result_json,omitempty"`
	Error         *ToolError `json:"error,omitempty"`
}

// PingMessage is a client keepalive.
type PingMessage struct {
	Type string `json:"type"`
}

// EventMessage is a server run event.
type EventMessage struct {
	Type      string          `json:"type"`
	RunID     string          `json:"run_id"`
	Attempt   int32           `json:"attempt"`
	Seq       int64           `json:"seq"`
	Source    string          `json:"source"`
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data,omitempty"`
	TSMillis  int64           `json:"ts_ms"`
}

// LocalToolCallMessage asks the App to execute a local tool.
type LocalToolCallMessage struct {
	Type          string `json:"type"`
	MsgID         string `json:"msg_id"`
	InvocationID  string `json:"invocation_id"`
	ToolName      string `json:"tool_name"`
	ArgumentsJSON string `json:"arguments_json"`
	TimeoutMS     int64  `json:"timeout_ms"`
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

func (*HelloMessage) isClientMessage()           {}
func (*SubscribeMessage) isClientMessage()       {}
func (*LocalToolResultMessage) isClientMessage() {}
func (*PingMessage) isClientMessage()            {}

func (*EventMessage) isServerMessage()         {}
func (*LocalToolCallMessage) isServerMessage() {}
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

// Validate checks the optional v1 handshake.
func (m HelloMessage) Validate() error {
	if m.Type != contract.WSMessageHello {
		return fmt.Errorf("hello.type must be %q", contract.WSMessageHello)
	}
	if m.Protocol != contract.WSProtocolVersion {
		return fmt.Errorf("unsupported ws protocol %d", m.Protocol)
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
