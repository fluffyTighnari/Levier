// Levier — internal/app/httpapi
//
// Request ID, body limit, and uniform JSON error tests.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fluffyTighnari/levier/internal/state/runstore"
)

func TestRequestIDEchoAndGenerate(t *testing.T) {
	_, _, server := newTestServer(t)

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/runs", strings.NewReader(`{`))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set(RequestIDHeader, "req-test-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get(RequestIDHeader); got != "req-test-1" {
		t.Fatalf("response request id = %q", got)
	}
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.RequestID != "req-test-1" {
		t.Fatalf("error request_id = %q", body.Error.RequestID)
	}

	resp = request(t, http.MethodGet, server.URL+"/healthz", "", nil)
	defer resp.Body.Close()
	if resp.Header.Get(RequestIDHeader) == "" {
		t.Fatal("generated request id is empty")
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/healthz", nil)
	req.Header.Set(RequestIDHeader, "bad id with spaces")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get(RequestIDHeader); got == "" || got == "bad id with spaces" {
		t.Fatalf("invalid request id was not replaced: %q", got)
	}
}

func TestBodyLimitAndUniformJSONErrors(t *testing.T) {
	store := newFakeStore()
	queue := &fakeQueue{}
	server := NewServer(store, queue, Config{BearerToken: "token-1", AllowLegacyBearer: true, MaxBodyBytes: 32})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "token-1", []byte(validBody))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, "REQUEST_TOO_LARGE")

	resp = request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "token-1", []byte(`{`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid JSON status = %d", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, "BAD_REQUEST")

	resp = request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "token-1", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body status = %d", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, "BAD_REQUEST")
}

func TestCancelInvalidJSONReturnsBadRequest(t *testing.T) {
	store := newFakeStore()
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: "app-1", Status: "running"}
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "token-1",
		AllowLegacyBearer: true,
		Canceller:         &fakeCanceller{run: store.runs["run-1"]},
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs/run-1/cancel", "token-1", []byte(`{`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, "BAD_REQUEST")
}

func assertErrorEnvelope(t *testing.T, resp *http.Response, code string) {
	t.Helper()
	defer resp.Body.Close()
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q", body.Error.Code, code)
	}
	if body.Error.RequestID == "" {
		t.Fatal("error request_id is empty")
	}
}
