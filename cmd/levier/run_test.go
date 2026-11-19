package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type runTestServer struct {
	mu        sync.Mutex
	gets      int
	status    string
	cancelled bool
}

func newRunTestServer(t *testing.T, terminalStatus string) (*httptest.Server, *runTestServer) {
	t.Helper()
	state := &runTestServer{status: "queued"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/runs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":   "run_1",
				"status":   "queued",
				"ws_url":   "/api/v1/ws",
				"ws_token": "token",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/run_1":
			state.gets++
			if state.gets > 1 {
				state.status = terminalStatus
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":       "run_1",
				"app_id":       "app_1",
				"runtime_name": "react1",
				"status":       state.status,
				"attempt":      1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/run_1/events":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{{
					"run_id":     "run_1",
					"attempt":    1,
					"seq":        2,
					"source":     "daemon",
					"event_type": "MESSAGE_COMPLETED",
					"data":       map[string]any{"content": "done"},
					"ts_ms":      1,
				}},
				"caught_up": true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/runs/run_1/cancel":
			state.cancelled = true
			state.status = "cancelled"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"run_id": "run_1", "status": "cancelled"})
		default:
			http.NotFound(w, r)
		}
	}))
	return server, state
}

func testCredentials(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"app_id":"app_1","app_name":"demo","api_key":"mw_test"}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	return path
}

func TestRunSubmitJSON(t *testing.T) {
	server, _ := newRunTestServer(t, "completed")
	defer server.Close()
	path := testCredentials(t)

	var stdout, stderr bytes.Buffer
	code := runRunSubmit([]string{
		"--credentials", path,
		"--server", server.URL,
		"--prompt", "hello",
		"--runtime", "react1",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run submit = %d, stderr = %s", code, stderr.String())
	}
	var result runSubmitResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode run submit: %v (%s)", err, stdout.String())
	}
	if result.RunID != "run_1" || result.Status != "queued" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunSubmitWatchJSON(t *testing.T) {
	server, _ := newRunTestServer(t, "completed")
	defer server.Close()
	path := testCredentials(t)

	var stdout, stderr bytes.Buffer
	code := runRunSubmit([]string{
		"--credentials", path,
		"--server", server.URL,
		"--prompt", "hello",
		"--runtime", "react1",
		"--watch",
		"--interval", "1ms",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run submit --watch = %d, stderr = %s", code, stderr.String())
	}
	var result runWatchResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode watch: %v (%s)", err, stdout.String())
	}
	if result.RunID != "run_1" || result.Status != "completed" || result.FinalMessage != "done" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunCancel(t *testing.T) {
	server, state := newRunTestServer(t, "completed")
	defer server.Close()
	path := testCredentials(t)

	var stdout, stderr bytes.Buffer
	code := runRunCancel([]string{
		"--credentials", path,
		"--server", server.URL,
		"--run-id", "run_1",
		"--yes",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run cancel = %d, stderr = %s", code, stderr.String())
	}
	if !state.cancelled {
		t.Fatal("server cancel was not called")
	}
	if !strings.Contains(stdout.String(), "cancelled") {
		t.Fatalf("cancel output = %s", stdout.String())
	}
}
