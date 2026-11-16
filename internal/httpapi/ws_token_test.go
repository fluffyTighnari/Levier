// Middleware — internal/httpapi
//
// WebSocket token, replay, and keepalive tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"github.com/fluffyTighnari/levier/internal/contract"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

func TestWSTokenExpiryAndReissue(t *testing.T) {
	store := newFakeStore()
	appHub := connection.NewAppHub()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		WSTokenTTL:        time.Minute,
		AppHub:            appHub,
	})
	now := time.Now()
	server.wsTokens.now = func() time.Time { return now }
	token := server.wsTokens.Issue("app-1")
	now = now.Add(2 * time.Minute)
	if _, ok := server.wsTokens.Validate(token); ok {
		t.Fatal("expired token was accepted")
	}

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + token
	if conn, err := websocket.Dial(wsURL, "", httpServer.URL); err == nil {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err == nil {
			t.Fatalf("expired token connection stayed open: %s", raw)
		}
		_ = conn.Close()
	}

	req, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/ws-token?app_id=app-1", nil)
	req.Header.Set("Authorization", "Bearer legacy-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ws-token Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ws-token status = %d", resp.StatusCode)
	}
	var refreshed WSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshed); err != nil {
		t.Fatalf("decode ws-token: %v", err)
	}
	if refreshed.WSToken == "" || refreshed.WSURL != WSURL || refreshed.ExpiresIn != 60 {
		t.Fatalf("ws-token = %+v", refreshed)
	}

	wsURL = "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-1&token=" + refreshed.WSToken
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		t.Fatalf("Dial(refreshed) error = %v", err)
	}
	defer conn.Close()

	if err := websocket.Message.Send(conn, `{"type":"ping"}`); err != nil {
		t.Fatalf("Send(ping) error = %v", err)
	}
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("Receive(pong) error = %v", err)
	}
	if !strings.Contains(raw, `"type":"pong"`) {
		t.Fatalf("pong = %s", raw)
	}
}

func TestWSTokenReissueWithAppKey(t *testing.T) {
	store, _, apps, server := newAppTestServer(t)
	app, key, _, _ := apps.RegisterApp(context.Background(), "app-token", false)
	_ = store

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ws-token", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var token WSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if token.WSToken == "" || token.ExpiresIn != 600 {
		t.Fatalf("token = %+v", token)
	}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") +
		"/api/v1/ws?app_id=" + app.AppID + "&token=" + token.WSToken
	conn, err := websocket.Dial(wsURL, "", server.URL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/ws-token", nil)
	req.Header.Set("Authorization", "Bearer legacy-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("legacy without app_id status = %d", resp.StatusCode)
	}
}

func TestWebSocketReplayPaginatesAndResumes(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-1", Status: "running"}
	const total = 1205
	for i := 1; i <= total; i++ {
		store.events = append(store.events, runstore.Event{
			RunID: "run-1", Attempt: 1, Seq: int64(i), Source: "daemon",
			Type: "MESSAGE_DELTA", Payload: json.RawMessage(`{}`), TSMs: int64(i),
		})
	}
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
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	subscribe, _ := json.Marshal(SubscribeMessage{
		Type: contract.WSMessageSubscribe, RunID: "run-1", LastAttempt: 1, LastSeq: 500,
	})
	if err := websocket.Message.Send(conn, string(subscribe)); err != nil {
		t.Fatalf("Send(subscribe) error = %v", err)
	}

	sequences := make([]int64, 0, total-500)
	var caughtUp CaughtUpMessage
	for {
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(raw), &probe); err != nil {
			t.Fatalf("decode message: %v", err)
		}
		if probe.Type == contract.WSMessageCaughtUp {
			if err := json.Unmarshal([]byte(raw), &caughtUp); err != nil {
				t.Fatalf("decode caught_up: %v", err)
			}
			break
		}
		if probe.Type != contract.WSMessageEvent {
			t.Fatalf("unexpected message: %s", raw)
		}
		var event EventMessage
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		sequences = append(sequences, event.Seq)
	}

	if len(sequences) != total-500 {
		t.Fatalf("replayed %d events, want %d", len(sequences), total-500)
	}
	for i, seq := range sequences {
		want := int64(501 + i)
		if seq != want {
			t.Fatalf("event[%d].seq = %d, want %d", i, seq, want)
		}
	}
	if caughtUp.RunID != "run-1" || caughtUp.Attempt != 1 || caughtUp.Seq != total {
		t.Fatalf("caught_up = %+v", caughtUp)
	}
}

func TestWebSocketTokenRejectsMismatchedApp(t *testing.T) {
	store := newFakeStore()
	server := NewServer(store, &fakeQueue{}, Config{BearerToken: "legacy-token", AllowLegacyBearer: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	token := server.wsTokens.Issue("app-1")
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/ws?app_id=app-2&token=" + token
	conn, err := websocket.Dial(wsURL, "", httpServer.URL)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err == nil {
		t.Fatalf("mismatched app connection stayed open: %s", raw)
	}
}

func TestWebSocketReplayLimitAndCursor(t *testing.T) {
	// Guard the replay page size boundary: exactly one full page plus one.
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-1", Status: "running"}
	for i := 1; i <= wsReplayPageSize+1; i++ {
		store.events = append(store.events, runstore.Event{
			RunID: "run-1", Attempt: 1, Seq: int64(i), Source: "daemon",
			Type: "MESSAGE_DELTA", Payload: json.RawMessage(`{}`), TSMs: int64(i),
		})
	}
	server := NewServer(store, &fakeQueue{}, Config{BearerToken: "legacy-token", AllowLegacyBearer: true})
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
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	subscribe, _ := json.Marshal(SubscribeMessage{Type: contract.WSMessageSubscribe, RunID: "run-1"})
	if err := websocket.Message.Send(conn, string(subscribe)); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	count := 0
	for {
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		if strings.Contains(raw, `"type":"caught_up"`) {
			break
		}
		count++
	}
	if count != wsReplayPageSize+1 {
		t.Fatalf("replayed %d events, want %d", count, wsReplayPageSize+1)
	}
}
