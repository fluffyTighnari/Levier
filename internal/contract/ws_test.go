// Levier — internal/contract
//
// Wire constants must not drift: clients and docs pin these strings.
//
// Copyright 2026 The Levier Authors.
package contract

import "testing"

func TestWSMessageConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"hello", WSMessageHello, "hello"},
		{"subscribe", WSMessageSubscribe, "subscribe"},
		{"local_tool_result", WSMessageLocalToolResult, "local_tool_result"},
		{"ping", WSMessagePing, "ping"},
		{"event", WSMessageEvent, "event"},
		{"local_tool_call", WSMessageLocalToolCall, "local_tool_call"},
		{"run_completed", WSMessageRunCompleted, "run_completed"},
		{"pong", WSMessagePong, "pong"},
		{"caught_up", WSMessageCaughtUp, "caught_up"},
		{"error", WSMessageError, "error"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if WSProtocolVersion != 1 {
		t.Fatalf("protocol version = %d, want 1", WSProtocolVersion)
	}
}
