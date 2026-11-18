// Levier — HTTP status overview tests.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	statuspkg "github.com/fluffyTighnari/levier/internal/status"
)

type fakeStatusProvider struct {
	snapshot statuspkg.Snapshot
}

func (f fakeStatusProvider) Snapshot(context.Context) statuspkg.Snapshot {
	return f.snapshot
}

func TestStatusEndpointIsPublicAndReturnsSnapshot(t *testing.T) {
	snapshot := statuspkg.New(statuspkg.Options{}).Snapshot(context.Background())
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{Status: fakeStatusProvider{snapshot: snapshot}})
	httpServer := newHTTPTestServer(t, server)

	resp := request(t, http.MethodGet, httpServer.URL+"/status", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /status status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var got statuspkg.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode /status: %v", err)
	}
	if got.StatusVersion != statuspkg.Version || got.Service != statuspkg.ServiceName {
		t.Fatalf("identity = %d/%q", got.StatusVersion, got.Service)
	}
}

func TestStatusEndpointDoesNotLeakIdentifiers(t *testing.T) {
	snapshot := statuspkg.New(statuspkg.Options{}).Snapshot(context.Background())
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{Status: fakeStatusProvider{snapshot: snapshot}})
	httpServer := newHTTPTestServer(t, server)
	resp := request(t, http.MethodGet, httpServer.URL+"/status", "", nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /status: %v", err)
	}
	text := string(body)
	for _, forbidden := range []string{"daemon_id", "app_id", "run_id", "admin-change-me", "postgres://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("/status leaked %q: %s", forbidden, text)
		}
	}
}

func TestStatusEndpointRequiresProvider(t *testing.T) {
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{})
	httpServer := newHTTPTestServer(t, server)
	resp := request(t, http.MethodGet, httpServer.URL+"/status", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /status without provider = %d, want 503", resp.StatusCode)
	}
}

func TestStatusEndpointRejectsWrites(t *testing.T) {
	snapshot := statuspkg.New(statuspkg.Options{}).Snapshot(context.Background())
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{Status: fakeStatusProvider{snapshot: snapshot}})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	resp := request(t, http.MethodPost, httpServer.URL+"/status", "", []byte(`{}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /status status = %d, want 405", resp.StatusCode)
	}
}
