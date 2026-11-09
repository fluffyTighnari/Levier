// Middleware — internal/httpapi
//
// Minimal App HTTP API tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

type fakeStore struct {
	run           runstore.Run
	runs          map[string]runstore.Run
	idempotency   map[string]runstore.Run
	events        []runstore.Event
	sessions      map[string]runstore.Session
	sessionEvents []runstore.Event
	lastAfterA    int32
	lastAfterS    int64
	lastLimit     int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		runs:        make(map[string]runstore.Run),
		idempotency: make(map[string]runstore.Run),
		sessions:    make(map[string]runstore.Session),
	}
}

func (f *fakeStore) CreateRun(_ context.Context, run runstore.Run) (runstore.Run, error) {
	if run.IdempotencyKey != "" {
		if existing, ok := f.idempotency[run.IdempotencyKey]; ok {
			if existing.RequestHash != run.RequestHash {
				return runstore.Run{}, runstore.ErrIdempotencyConflict
			}
			return existing, nil
		}
	}
	run.CreatedAt = time.Now()
	run.UpdatedAt = run.CreatedAt
	f.runs[run.RunID] = run
	if run.IdempotencyKey != "" {
		f.idempotency[run.IdempotencyKey] = run
	}
	return run, nil
}

func (f *fakeStore) GetRun(_ context.Context, runID string) (runstore.Run, error) {
	run, ok := f.runs[runID]
	if !ok {
		return runstore.Run{}, runstore.ErrRunNotFound
	}
	return run, nil
}

func (f *fakeStore) ListEvents(_ context.Context, _ string, afterAttempt int32, afterSeq int64, limit int) ([]runstore.Event, error) {
	f.lastAfterA = afterAttempt
	f.lastAfterS = afterSeq
	f.lastLimit = limit
	result := make([]runstore.Event, 0, len(f.events))
	for _, event := range f.events {
		if int32(event.Attempt) > afterAttempt ||
			(int32(event.Attempt) == afterAttempt && event.Seq > afterSeq) {
			result = append(result, event)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) GetSession(_ context.Context, sessionID string) (runstore.Session, error) {
	session, ok := f.sessions[sessionID]
	if !ok {
		return runstore.Session{}, runstore.ErrSessionNotFound
	}
	return session, nil
}

func (f *fakeStore) ListSessionEvents(_ context.Context, _ string, afterAttempt int32, afterSeq int64, limit int) ([]runstore.Event, error) {
	f.lastAfterA = afterAttempt
	f.lastAfterS = afterSeq
	f.lastLimit = limit
	return f.sessionEvents, nil
}

type fakeCanceller struct {
	run    runstore.Run
	reason string
}

func (f *fakeCanceller) CancelRun(_ context.Context, _ string, reason string) (runstore.Run, error) {
	f.reason = reason
	f.run.Status = "cancelled"
	return f.run, nil
}

type fakeQueue struct {
	tasks []workflow.Task
	err   error
}

func (f *fakeQueue) Enqueue(_ context.Context, task workflow.Task) (workflow.Task, error) {
	if f.err != nil {
		return workflow.Task{}, f.err
	}
	f.tasks = append(f.tasks, task)
	return task, nil
}

const validBody = `{
  "app_id": "app-1",
  "runtime": "normal",
  "strategy": {"name": "echo", "params": {"max_steps": 8}},
  "input": [{"role": "user", "content": "hello"}],
  "model": {"provider": "anthropic", "model": "claude-test"},
  "timeouts": {"run_ms": 1000, "tool_ms": 500}
}`

func newTestServer(t *testing.T) (*fakeStore, *fakeQueue, *httptest.Server) {
	t.Helper()
	store := newFakeStore()
	queue := &fakeQueue{}
	server := NewServer(store, queue, Config{BearerToken: "token-1", AllowLegacyBearer: true})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return store, queue, httpServer
}

func request(t *testing.T, method, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	return resp
}

func TestCreateRunGetRunAndEvents(t *testing.T) {
	store, queue, server := newTestServer(t)
	resp := request(t, http.MethodPost, server.URL+"/api/v1/runs", "token-1", []byte(validBody))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST status = %d", resp.StatusCode)
	}
	var created CreateRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	if created.RunID == "" || created.Status != RunStatusQueued {
		t.Fatalf("create response = %+v", created)
	}
	if len(queue.tasks) != 2 || queue.tasks[0].RunID != created.RunID {
		t.Fatalf("tasks = %+v", queue.tasks)
	}
	if queue.tasks[0].Type != workflow.TaskDispatch || queue.tasks[1].Type != workflow.TaskTimeout {
		t.Fatalf("task types = %v/%v", queue.tasks[0].Type, queue.tasks[1].Type)
	}
	stored := store.runs[created.RunID]
	if stored.RunTimeoutMS != 1000 || stored.ToolTimeoutMS != 500 {
		t.Fatalf("timeouts = %d/%d", stored.RunTimeoutMS, stored.ToolTimeoutMS)
	}

	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/"+created.RunID, "token-1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET run status = %d", resp.StatusCode)
	}
	var view RunView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	resp.Body.Close()
	if view.RunID != created.RunID || view.Status != RunStatusQueued || view.Strategy.Name != "echo" {
		t.Fatalf("view = %+v", view)
	}

	store.events = []runstore.Event{{
		RunID: created.RunID, Attempt: 2, Seq: 3, Source: "daemon",
		Type: "RUN_STATUS", Payload: json.RawMessage(`{"status":"running"}`), TSMs: 10,
	}}
	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/"+created.RunID+"/events?after_attempt=1&after_seq=2&limit=10", "token-1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET events status = %d", resp.StatusCode)
	}
	var events EventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	resp.Body.Close()
	if len(events.Events) != 1 || events.Events[0].EventType != "RUN_STATUS" {
		t.Fatalf("events = %+v", events)
	}
	if store.lastAfterA != 1 || store.lastAfterS != 2 || store.lastLimit != 10 {
		t.Fatalf("query = %d/%d/%d", store.lastAfterA, store.lastAfterS, store.lastLimit)
	}
}

func TestAuthAndValidation(t *testing.T) {
	_, _, server := newTestServer(t)

	resp := request(t, http.MethodPost, server.URL+"/api/v1/runs", "", []byte(validBody))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodPost, server.URL+"/api/v1/runs", "wrong", []byte(validBody))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong auth status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodPost, server.URL+"/api/v1/runs", "token-1", []byte(`{"runtime":"normal"}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid body status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/missing", "token-1", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing run status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/healthz", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestIdempotentCreate(t *testing.T) {
	store, queue, server := newTestServer(t)

	post := func(body string, key string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/runs", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("Authorization", "Bearer token-1")
		req.Header.Set("Idempotency-Key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		return resp
	}

	first := post(validBody, "key-1")
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first status = %d", first.StatusCode)
	}
	var firstResponse CreateRunResponse
	_ = json.NewDecoder(first.Body).Decode(&firstResponse)
	first.Body.Close()

	second := post(validBody, "key-1")
	if second.StatusCode != http.StatusAccepted {
		t.Fatalf("second status = %d", second.StatusCode)
	}
	var secondResponse CreateRunResponse
	_ = json.NewDecoder(second.Body).Decode(&secondResponse)
	second.Body.Close()
	if firstResponse.RunID != secondResponse.RunID {
		t.Fatalf("run IDs differ: %s vs %s", firstResponse.RunID, secondResponse.RunID)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(queue.tasks))
	}
	if len(store.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(store.runs))
	}

	third := post(validBody+" ", "key-1")
	if third.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting status = %d", third.StatusCode)
	}
	third.Body.Close()
}

func TestCancelRun(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-1", Status: "running"}
	canceller := &fakeCanceller{run: store.runs["run-1"]}
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "token-1",
		AllowLegacyBearer: true,
		Canceller:         canceller,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs/run-1/cancel", "token-1", []byte(`{"reason":"user"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var cancelled CancelRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&cancelled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if cancelled.Status != RunStatusCancelled || canceller.reason != "user" {
		t.Fatalf("cancelled = %+v reason=%q", cancelled, canceller.reason)
	}
}

func TestQueueFailureReturns500(t *testing.T) {
	store := newFakeStore()
	queue := &fakeQueue{err: errors.New("queue down")}
	server := NewServer(store, queue, Config{BearerToken: "token-1", AllowLegacyBearer: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "token-1", []byte(validBody))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}
