// Levier — internal/httpapi
//
// Longterm session HTTP tests.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fluffyTighnari/levier/internal/runstore"
)

func TestSessionRESTAndAppScope(t *testing.T) {
	store, _, apps, server := newAppTestServer(t)
	app, key, _, _ := apps.RegisterApp(context.Background(), "app-session", false)
	other, otherKey, _, _ := apps.RegisterApp(context.Background(), "app-other", false)

	expires := time.Now().Add(time.Hour)
	store.sessions["session-1"] = runstore.Session{
		SessionID:      "session-1",
		AppID:          app.AppID,
		Status:         "active",
		CurrentRunID:   "run-9",
		EventAttempt:   2,
		EventWatermark: 11,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ExpiresAt:      &expires,
	}

	resp := request(t, http.MethodGet, server.URL+"/api/v1/sessions/session-1", key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own session status = %d", resp.StatusCode)
	}
	var session SessionView
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	resp.Body.Close()
	if session.SessionID != "session-1" || session.AppID != app.AppID ||
		session.EventAttempt != 2 || session.EventWatermark != 11 || session.ExpiresAt == nil {
		t.Fatalf("session = %+v", session)
	}

	resp = request(t, http.MethodGet, server.URL+"/api/v1/sessions/session-1", otherKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other app status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
	_ = other

	resp = request(t, http.MethodGet, server.URL+"/api/v1/sessions/session-1", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/api/v1/sessions/missing", key, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp.StatusCode)
	}
	var body ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Error.Code != "SESSION_NOT_FOUND" {
		t.Fatalf("missing error = %+v", body.Error)
	}
}

func TestSessionEventsREST(t *testing.T) {
	store, _, apps, server := newAppTestServer(t)
	app, key, _, _ := apps.RegisterApp(context.Background(), "app-events", false)
	store.sessions["session-1"] = runstore.Session{SessionID: "session-1", AppID: app.AppID, Status: "active"}
	store.sessionEvents = []runstore.Event{
		{RunID: "run-1", Attempt: 1, Seq: 3, Source: "daemon", Type: "RUN_STATUS", Payload: json.RawMessage(`{"status":"running"}`), TSMs: 10},
	}

	resp := request(t, http.MethodGet,
		server.URL+"/api/v1/sessions/session-1/events?after_attempt=1&after_seq=2&limit=10", key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var events EventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	resp.Body.Close()
	if len(events.Events) != 1 || events.Events[0].EventType != "RUN_STATUS" ||
		events.NextAttempt != 1 || events.NextSeq != 3 {
		t.Fatalf("events = %+v", events)
	}
	if store.lastAfterA != 1 || store.lastAfterS != 2 || store.lastLimit != 10 {
		t.Fatalf("query = %d/%d/%d", store.lastAfterA, store.lastAfterS, store.lastLimit)
	}
}

func TestSessionStoreNotConfigured(t *testing.T) {
	store := newFakeStore()
	server := NewServer(runOnlyStore{store: store}, &fakeQueue{}, Config{BearerToken: "token-1", AllowLegacyBearer: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodGet, httpServer.URL+"/api/v1/sessions/session-1", "token-1", nil)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// runOnlyStore hides the optional SessionStore methods.
type runOnlyStore struct {
	store *fakeStore
}

func (s runOnlyStore) CreateRun(ctx context.Context, run runstore.Run) (runstore.Run, error) {
	return s.store.CreateRun(ctx, run)
}

func (s runOnlyStore) GetRun(ctx context.Context, runID string) (runstore.Run, error) {
	return s.store.GetRun(ctx, runID)
}

func (s runOnlyStore) ListEvents(ctx context.Context, runID string, afterAttempt int32, afterSeq int64, limit int) ([]runstore.Event, error) {
	return s.store.ListEvents(ctx, runID, afterAttempt, afterSeq, limit)
}
