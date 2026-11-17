// Package contract holds wire-level constants shared by the HTTP/WS surface.
//
// It must not import any other internal package.
//
// Levier — internal/contract
// Copyright 2026 The Levier Authors.
package contract

const (
	// WSProtocolVersion is the v1 App WebSocket protocol version.
	WSProtocolVersion = 1
)

const (
	// WSMessageHello is the optional client handshake.
	WSMessageHello = "hello"
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
	// WSMessageRunCompleted announces a terminal run.
	WSMessageRunCompleted = "run_completed"
	// WSMessagePong is a server keepalive.
	WSMessagePong = "pong"
	// WSMessageCaughtUp marks replay completion.
	WSMessageCaughtUp = "caught_up"
	// WSMessageError is a server error.
	WSMessageError = "error"
)
