// Levier — self-check tests.
//
// Copyright 2026 The Levier Authors.
package selfcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/ops/selfdesc"
	statuspkg "github.com/fluffyTighnari/levier/internal/ops/status"
)

func TestRunnerHTTPHops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz", "/admin/mcp":
			if r.URL.Path == "/admin/mcp" && r.Header.Get("Authorization") != "Bearer admin-test" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/meta":
			_ = json.NewEncoder(w).Encode(selfdesc.New(selfdesc.Options{Version: "v-test"}))
		case "/status":
			_ = json.NewEncoder(w).Encode(statuspkg.New(statuspkg.Options{
				Health: func(context.Context) error { return nil },
			}).Snapshot(context.Background()))
		case "/api/v1/apps/app-1":
			if r.Header.Get("Authorization") != "Bearer app-key" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	r := &runner{
		opts: Options{
			Server:     server.URL,
			AdminToken: "admin-test",
			Timeout:    5 * time.Second,
		},
		client: server.Client(),
	}
	if hop := r.checkHTTP("/healthz", "health"); hop.Status != StatusOK {
		t.Fatalf("health hop = %+v", hop)
	}
	if hop := r.checkMeta(); hop.Status != StatusOK {
		t.Fatalf("meta hop = %+v", hop)
	}
	if hop := r.checkStatus(); hop.Status != StatusOK {
		t.Fatalf("status hop = %+v", hop)
	}
	if hop := r.checkAdmin(); hop.Status != StatusOK {
		t.Fatalf("admin hop = %+v", hop)
	}
	r.status = statuspkg.New(statuspkg.Options{
		Health: func(context.Context) error { return nil },
		Hub:    fakeDaemonSource{connected: 1},
	}).Snapshot(context.Background())
	r.haveStatus = true
	if hop := r.checkDaemon(); hop.Status != StatusOK {
		t.Fatalf("daemon hop = %+v", hop)
	}

	credentialPath := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(credentialPath, []byte(`{"app_id":"app-1","api_key":"app-key"}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	r.opts.Credentials = credentialPath
	if hop := r.checkApp(); hop.Status != StatusOK {
		t.Fatalf("app hop = %+v", hop)
	}
}

type fakeDaemonSource struct{ connected int }

func (f fakeDaemonSource) List() []connection.DaemonInfo {
	return []connection.DaemonInfo{{Healthy: true, MaxConcurrentRuns: 1}}
}

func TestRunReportContainsOrderedHops(t *testing.T) {
	report := Run(Options{
		ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		Server:     "http://127.0.0.1:1",
		Timeout:    50 * time.Millisecond,
	})
	if len(report.Hops) < 10 {
		t.Fatalf("hops = %d", len(report.Hops))
	}
	if report.Hops[0].Name != "config" || report.Hops[1].Name != "http /healthz" {
		t.Fatalf("hop order = %+v", report.Hops[:2])
	}
	if report.OK {
		t.Fatal("report.OK = true, want false")
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	for _, secret := range []string{"admin-test", "app-key", "change-me"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("report leaked %q: %s", secret, data)
		}
	}
}

func TestPublicSelfcheckSchemaAndExample(t *testing.T) {
	schemaData, err := os.ReadFile("../../../public/service-selfcheck/v1/schema.json")
	if err != nil {
		t.Fatalf("read selfcheck schema: %v", err)
	}
	if !json.Valid(schemaData) {
		t.Fatal("selfcheck schema is not valid JSON")
	}
	exampleData, err := os.ReadFile("../../../public/service-selfcheck/v1/example.json")
	if err != nil {
		t.Fatalf("read selfcheck example: %v", err)
	}
	var example Report
	if err := json.Unmarshal(exampleData, &example); err != nil {
		t.Fatalf("decode selfcheck example: %v", err)
	}
	if example.Server == "" || example.Hops == nil {
		t.Fatalf("example = %+v", example)
	}
}
