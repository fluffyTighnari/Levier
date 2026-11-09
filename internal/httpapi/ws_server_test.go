// Middleware — internal/httpapi
//
// App WebSocket subscription and fanout tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/runstore"
)

func TestWebSocketSubscribeAndPublish(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{
		RunID: "run-1", AppID: "app-1", Status: "running",
	}
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		WSTokenTTL:        time.Minute,
		AppHub:            appHub,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	token := server.wsTokens.Issue("app-1")
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + token
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	subscribe, _ := json.Marshal(SubscribeMessage{Type: WSMessageSubscribe, RunID: "run-1"})
	if err := websocket.Message.Send(conn, string(subscribe)); err != nil {
		t.Fatalf("Send(subscribe) error = %v", err)
	}
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(caught_up) error = %v", err)
	}
	var caughtUp CaughtUpMessage
	if err := json.Unmarshal([]byte(raw), &caughtUp); err != nil || caughtUp.Type != WSMessageCaughtUp {
		t.Fatalf("caught_up = %s, %v", raw, err)
	}

	appHub.Publish("app-1", "run-1", []byte(`{"type":"event","run_id":"run-1","seq":1}`))
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(event) error = %v", err)
	}
	if !strings.Contains(raw, `"type":"event"`) {
		t.Fatalf("event = %s", raw)
	}
}

func TestWebSocketRoutesLocalToolResult(t *testing.T) {
	store := newFakeStore()
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AppHub:            appHub,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	token := server.wsTokens.Issue("app-1")
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + token
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(time.Second)
	for appHub.Count("app-1") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	resultCh := make(chan connection.LocalToolResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := appHub.CallLocalTool(context.Background(), "app-1", connection.LocalToolRequest{
			InvocationID: "inv-1", ToolName: "search", ArgumentsJSON: `{"q":"x"}`, TimeoutMS: 1000,
		})
		resultCh <- result
		errCh <- err
	}()

	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(local_tool_call) error = %v", err)
	}
	var call LocalToolCallMessage
	if err := json.Unmarshal([]byte(raw), &call); err != nil || call.Type != WSMessageLocalToolCall {
		t.Fatalf("local tool call = %s, %v", raw, err)
	}
	result, _ := json.Marshal(LocalToolResultMessage{
		Type:          WSMessageLocalToolResult,
		CorrelationID: call.MsgID,
		InvocationID:  "inv-1",
		OK:            true,
		ResultJSON:    "3",
	})
	if err := websocket.Message.Send(conn, string(result)); err != nil {
		t.Fatalf("Send(local_tool_result) error = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	if got := <-resultCh; !got.OK || got.ResultJSON != "3" {
		t.Fatalf("result = %+v", got)
	}
}

func TestWebSocketRejectsWrongToken(t *testing.T) {
	store := newFakeStore()
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AppHub:            appHub,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=bad"
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err == nil {
		t.Fatalf("bad token connection remained open: %s", raw)
	}
}

func TestWebSocketRejectsForeignRunSubscription(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-2"}
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AppHub:            appHub,
	})
	server.wsTokens.now = func() time.Time { return time.Now() }
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	token := server.wsTokens.Issue("app-1")
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + token
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	subscribe, _ := json.Marshal(SubscribeMessage{Type: WSMessageSubscribe, RunID: "run-1"})
	_ = websocket.Message.Send(conn, string(subscribe))
	var raw string
	_ = websocket.Message.Receive(conn, &raw)
	if !strings.Contains(raw, "RUN_NOT_FOUND") {
		t.Fatalf("subscribe error = %s", raw)
	}
}

func TestWebSocketLocalToolValidationAndIdempotency(t *testing.T) {
	store := newFakeStore()
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AppHub:            appHub,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	dial := func(appID string) *websocket.Conn {
		t.Helper()
		token := server.wsTokens.Issue(appID)
		wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
			"/api/v1/ws?app_id=" + appID + "&token=" + token
		conn, err := websocket.Dial(wsURL, "", httpServer.URL)
		if err != nil {
			t.Fatalf("Dial(%s) error = %v", appID, err)
		}
		return conn
	}
	receive := func(conn *websocket.Conn) string {
		t.Helper()
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		return raw
	}
	send := func(conn *websocket.Conn, message any) {
		t.Helper()
		payload, _ := json.Marshal(message)
		if err := websocket.Message.Send(conn, string(payload)); err != nil {
			t.Fatalf("Send() error = %v", err)
		}
	}

	first := dial("app-1")
	defer first.Close()
	second := dial("app-2")
	defer second.Close()
	deadline := time.Now().Add(time.Second)
	for appHub.Count("app-1") == 0 || appHub.Count("app-2") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("connections were not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}

	resultCh := make(chan connection.LocalToolResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := appHub.CallLocalTool(context.Background(), "app-1", connection.LocalToolRequest{
			InvocationID: "inv-1", ToolName: "search", ArgumentsJSON: `{"q":"x"}`, TimeoutMS: 2000,
		})
		resultCh <- result
		errCh <- err
	}()

	var call LocalToolCallMessage
	if err := json.Unmarshal([]byte(receive(first)), &call); err != nil || call.Type != WSMessageLocalToolCall {
		t.Fatalf("local tool call = %+v, %v", call, err)
	}

	send(first, LocalToolResultMessage{
		Type: WSMessageLocalToolResult, CorrelationID: call.MsgID,
		InvocationID: "missing", OK: true,
	})
	if got := receive(first); !strings.Contains(got, "LOCAL_TOOL_UNKNOWN") {
		t.Fatalf("unknown result = %s", got)
	}

	send(first, LocalToolResultMessage{
		Type: WSMessageLocalToolResult, CorrelationID: "wrong",
		InvocationID: "inv-1", OK: true,
	})
	if got := receive(first); !strings.Contains(got, "LOCAL_TOOL_CORRELATION_MISMATCH") {
		t.Fatalf("mismatch result = %s", got)
	}

	send(second, LocalToolResultMessage{
		Type: WSMessageLocalToolResult, CorrelationID: call.MsgID,
		InvocationID: "inv-1", OK: true,
	})
	if got := receive(second); !strings.Contains(got, "LOCAL_TOOL_FORBIDDEN") {
		t.Fatalf("forged result = %s", got)
	}

	send(first, LocalToolResultMessage{
		Type: WSMessageLocalToolResult, CorrelationID: call.MsgID,
		InvocationID: "inv-1", OK: true, ResultJSON: "3",
	})
	if err := <-errCh; err != nil {
		t.Fatalf("CallLocalTool() error = %v", err)
	}
	if result := <-resultCh; !result.OK || result.ResultJSON != "3" || result.CorrelationID != call.MsgID {
		t.Fatalf("result = %+v", result)
	}

	send(first, LocalToolResultMessage{
		Type: WSMessageLocalToolResult, CorrelationID: call.MsgID,
		InvocationID: "inv-1", OK: true, ResultJSON: "3",
	})
	send(first, PingMessage{Type: WSMessagePing})
	if got := receive(first); !strings.Contains(got, `"type":"pong"`) {
		t.Fatalf("duplicate result produced an error: %s", got)
	}
}

func TestWebSocketRunCompleted(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-1", Status: "running"}
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AppHub:            appHub,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	token := server.wsTokens.Issue("app-1")
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + token
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	subscribe, _ := json.Marshal(SubscribeMessage{Type: WSMessageSubscribe, RunID: "run-1"})
	if err := websocket.Message.Send(conn, string(subscribe)); err != nil {
		t.Fatalf("Send(subscribe) error = %v", err)
	}
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(caught_up) error = %v", err)
	}

	appHub.Publish("app-1", "run-1", []byte(`{"type":"run_completed","run_id":"run-1","status":"completed","attempt":1}`))
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(run_completed) error = %v", err)
	}
	if !strings.Contains(raw, `"type":"run_completed"`) || !strings.Contains(raw, `"status":"completed"`) {
		t.Fatalf("run_completed = %s", raw)
	}
}
