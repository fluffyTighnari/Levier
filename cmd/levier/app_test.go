package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newAppTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/apps":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"app": map[string]any{
					"app_id":     "app_1",
					"name":       "demo",
					"status":     "active",
					"multiagent": false,
				},
				"api_key": "secret-key",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/apps/app_1/rotate-key":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"api_key": "new-secret-key"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app_1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"app_id":     "app_1",
				"name":       "demo",
				"status":     "active",
				"multiagent": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ws-token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ws_url":     "/api/v1/ws",
				"ws_token":   "ws-token",
				"expires_in": 60,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/apps":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"apps": []map[string]any{{
					"app_id":     "app_1",
					"name":       "demo",
					"status":     "active",
					"multiagent": false,
					"created_at": "2026-11-19T00:00:00Z",
					"updated_at": "2026-11-19T00:00:00Z",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRunAppInitShowRotate(t *testing.T) {
	server := newAppTestServer(t)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")

	var stdout, stderr bytes.Buffer
	code := runAppInit([]string{
		"--name", "demo",
		"--credentials-out", path,
		"--server", server.URL,
		"--admin-token", "admin",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("app init = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret-key") {
		t.Fatalf("app init leaked key: %s", stdout.String())
	}
	credentials, err := readCredentials(path)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if credentials.AppID != "app_1" || credentials.APIKey != "secret-key" {
		t.Fatalf("credentials = %+v", credentials)
	}

	stdout.Reset()
	stderr.Reset()
	code = runAppShow([]string{
		"--credentials", path,
		"--server", server.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("app show = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "app_1") {
		t.Fatalf("app show output = %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runAppRotateKey([]string{
		"--credentials", path,
		"--server", server.URL,
		"--admin-token", "admin",
		"--yes",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("app rotate-key = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "new-secret-key") {
		t.Fatalf("app rotate-key leaked key: %s", stdout.String())
	}
	credentials, err = readCredentials(path)
	if err != nil {
		t.Fatalf("read rotated credentials: %v", err)
	}
	if credentials.APIKey != "new-secret-key" {
		t.Fatalf("rotated api key = %q", credentials.APIKey)
	}
}

func TestRunAppCheckAndList(t *testing.T) {
	server := newAppTestServer(t)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")

	var stdout, stderr bytes.Buffer
	if code := runAppInit([]string{
		"--name", "demo",
		"--credentials-out", path,
		"--server", server.URL,
		"--admin-token", "admin",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("app init = %d, stderr = %s", code, stderr.String())
	}
	credentials, err := readCredentials(path)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if credentials.CredentialVersion != credentialVersion || credentials.Server != server.URL {
		t.Fatalf("credentials = %+v", credentials)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runAppCheck([]string{"--credentials", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("app check = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), credentials.APIKey) || !strings.Contains(stdout.String(), "app check: PASS") {
		t.Fatalf("app check output = %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runAppCheck([]string{"--credentials", path, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("app check --json = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), credentials.APIKey) || !strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("app check JSON = %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runAppList([]string{"--server", server.URL, "--admin-token", "admin", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("app list = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "app_1") {
		t.Fatalf("app list output = %s", stdout.String())
	}
}

func TestRunAppInitUsesAdminTokenEnv(t *testing.T) {
	server := newAppTestServer(t)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("LEVIER_ADMIN_TOKEN", "admin")

	var stdout, stderr bytes.Buffer
	code := runAppInit([]string{
		"--name", "demo",
		"--credentials-out", path,
		"--server", server.URL,
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("app init = %d, stderr = %s", code, stderr.String())
	}
	credentials, err := readCredentials(path)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if credentials.APIKey != "secret-key" {
		t.Fatalf("api key = %q", credentials.APIKey)
	}
}

func TestRunAppInitDryRunNeedsNoToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	var stdout, stderr bytes.Buffer
	code := runAppInit([]string{
		"--name", "demo",
		"--credentials-out", path,
		"--dry-run",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("app init --dry-run = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would create app") {
		t.Fatalf("dry-run output = %s", stdout.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote credentials: %v", err)
	}
}

func TestRunAppInitRefusesExistingCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"app_id":"app_1","api_key":"x"}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runAppInit([]string{
		"--name", "demo",
		"--credentials-out", path,
		"--server", "http://127.0.0.1:1",
		"--admin-token", "admin",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("app init existing = %d, want 2", code)
	}
}
