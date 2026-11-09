// Middleware — internal/httpapi
//
// App WebSocket subscription and fanout tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
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
		BearerToken: "legacy-token",
		WSTokenTTL:  time.Minute,
		AppHub:      appHub,
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

func TestWebSocketRejectsWrongToken(t *testing.T) {
	store := newFakeStore()
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken: "legacy-token",
		AppHub:      appHub,
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
		BearerToken: "legacy-token",
		AppHub:      appHub,
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
