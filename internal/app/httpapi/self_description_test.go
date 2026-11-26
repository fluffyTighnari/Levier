// Levier — HTTP self-description tests.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fluffyTighnari/levier/internal/ops/selfdesc"
)

func newHTTPTestServer(t *testing.T, server *Server) *httptest.Server {
	t.Helper()
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer
}

func TestMetaEndpointIsPublicAndDescribesService(t *testing.T) {
	_, _, server := newTestServer(t)
	resp := request(t, http.MethodGet, server.URL+"/meta", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /meta status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var doc selfdesc.Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /meta: %v", err)
	}
	if doc.MetaVersion != selfdesc.MetaVersion || doc.Service != selfdesc.ServiceName {
		t.Fatalf("identity = %d/%q", doc.MetaVersion, doc.Service)
	}
	if doc.API.HTTPVersion != "v1" || doc.API.WSProtocol != 1 {
		t.Fatalf("api = %+v", doc.API)
	}
	if doc.Endpoints.Runs != "/api/v1/runs" || doc.Endpoints.WebSocket != "/api/v1/ws" {
		t.Fatalf("endpoints = %+v", doc.Endpoints)
	}
}

func TestMetaUsesEffectiveLimits(t *testing.T) {
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{
		MaxBodyBytes:         1234,
		DefaultRunTimeoutMS:  5500,
		DefaultToolTimeoutMS: 2500,
		WSTokenTTL:           90 * time.Second,
	})
	httpServer := newHTTPTestServer(t, server)
	resp := request(t, http.MethodGet, httpServer.URL+"/meta", "", nil)
	defer resp.Body.Close()
	var doc selfdesc.Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /meta: %v", err)
	}
	if doc.Limits.MaxBodyBytes != 1234 ||
		doc.Limits.DefaultRunTimeoutMS != 5500 ||
		doc.Limits.DefaultToolTimeoutMS != 2500 ||
		doc.Limits.WSTokenTTLSeconds != 90 {
		t.Fatalf("limits = %+v", doc.Limits)
	}
}

func TestMetaDoesNotLeakSecrets(t *testing.T) {
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{
		BearerToken: "legacy-secret-token",
		AdminToken:  "admin-secret-token",
	})
	httpServer := newHTTPTestServer(t, server)
	resp := request(t, http.MethodGet, httpServer.URL+"/meta", "", nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /meta: %v", err)
	}
	text := string(body)
	for _, secret := range []string{"legacy-secret-token", "admin-secret-token", "change-me", "postgres://"} {
		if strings.Contains(text, secret) {
			t.Fatalf("/meta leaked %q: %s", secret, text)
		}
	}
}

func TestMetaRejectsWrites(t *testing.T) {
	_, _, server := newTestServer(t)
	resp := request(t, http.MethodPost, server.URL+"/meta", "", []byte(`{}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /meta status = %d, want 405", resp.StatusCode)
	}
}
