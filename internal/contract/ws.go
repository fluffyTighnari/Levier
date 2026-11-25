// Package contract holds wire-level constants shared by the HTTP/WS surface.
//
// It must not import any other internal package.
//
// Levier — internal/contract
// Copyright 2026 The Levier Authors.
package contract

const (
	// WSProtocolVersion is the legacy v1 App WebSocket protocol version.
	WSProtocolVersion = 1
	// WSProtocolVersionV2 is the additive App Bridge v2 protocol version.
	WSProtocolVersionV2 = 2
)

const (
	// WSCapabilityToolCallV2 advertises run/deadline-aware local tool calls.
	WSCapabilityToolCallV2 = "tool_call_v2"
	// WSCapabilityProgress advertises local tool progress messages.
	WSCapabilityProgress = "tool_progress"
	// WSCapabilityCancel advertises local tool cancellation messages.
	WSCapabilityCancel = "tool_cancel"
	// WSCapabilityIdempotentResult advertises idempotent result dedupe.
	WSCapabilityIdempotentResult = "idempotent_result"
	// WSCapabilityResumeCursor advertises cursor-based reconnect replay.
	WSCapabilityResumeCursor = "resume_cursor"
)

const (
	// WSMessageHello is the optional client handshake.
	WSMessageHello = "hello"
	// WSMessageHelloAck is the v2 server handshake response.
	WSMessageHelloAck = "hello_ack"
	// WSMessageSubscribe subscribes to a run.
	WSMessageSubscribe = "subscribe"
	// WSMessageLocalToolResult returns a local tool result.
	WSMessageLocalToolResult = "local_tool_result"
	// WSMessagePing is a client keepalive.
	WSMessagePing = "ping"
	// WSMessageEvent is a server run event.
	WSMessageEvent = "event"
	// WSMessageLocalToolCall asks the App to execute a local tool.
	WSMessageLocalToolCall = "local_tool_call"
	// WSMessageLocalToolProgress reports progress for one local tool call.
	WSMessageLocalToolProgress = "local_tool_progress"
	// WSMessageCancel asks the App to cancel one local tool call.
	WSMessageCancel = "cancel"
	// WSMessageRunCompleted announces a terminal run.
	WSMessageRunCompleted = "run_completed"
	// WSMessagePong is a server keepalive.
	WSMessagePong = "pong"
	// WSMessageCaughtUp marks replay completion.
	WSMessageCaughtUp = "caught_up"
	// WSMessageError is a server error.
	WSMessageError = "error"
)
