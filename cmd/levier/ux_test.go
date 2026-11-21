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

func TestRunOnceUsesProfileDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	server, _ := newRunTestServer(t, "completed")
	defer server.Close()
	credentials := testCredentials(t)
	if _, err := saveProfile(cliProfile{
		Server:      server.URL,
		Credentials: credentials,
		Runtime:     "echo1",
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"run", "hello", "--interval", "1ms"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "final: done") {
		t.Fatalf("run output = %s", stdout.String())
	}
}

func TestInitWritesDefaultsWithoutCredentialFlags(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	t.Setenv("LEVIER_ADMIN_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/apps":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"app":     map[string]any{"app_id": "app_1", "name": "demo", "status": "active"},
				"api_key": "mw_test",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app_1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"app_id": "app_1", "name": "demo", "status": "active",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if _, err := saveProfile(cliProfile{Server: server.URL}); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"init", "demo", "--runtime", "react1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ready: demo") {
		t.Fatalf("init output = %s", stdout.String())
	}
	profile := loadProfile()
	if profile.AppName != "demo" || profile.Runtime != "react1" {
		t.Fatalf("profile = %+v", profile)
	}
	if _, err := os.Stat(filepath.Join(home, "demo.json")); err != nil {
		t.Fatalf("credentials not written: %v", err)
	}
}

func TestRunOnceSplitsPromptFromFlags(t *testing.T) {
	flags, prompt := splitRunArgs([]string{"add 2", "--with-app", "--app", "app.py", "--json"})
	if prompt != "add 2" {
		t.Fatalf("prompt = %q", prompt)
	}
	joined := strings.Join(flags, " ")
	for _, want := range []string{"--with-app", "--app app.py", "--json"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("flags = %v", flags)
		}
	}
}

func TestRunShowAcceptsPositionalID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	server, _ := newRunTestServer(t, "completed")
	defer server.Close()
	if _, err := saveProfile(cliProfile{
		Server:      server.URL,
		Credentials: testCredentials(t),
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"run", "show", "run_1", "--events"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run show = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "MESSAGE_COMPLETED") {
		t.Fatalf("show output = %s", stdout.String())
	}
}

func TestUpDryRunStartsCoreServices(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LEVIER_HOME", t.TempDir())
	t.Setenv("LEVIER_STACK_FILE", "")
	if err := os.WriteFile("docker-compose.yml", []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"up", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("up --dry-run = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"docker compose", "up", "--build", "postgres", "react-runtime", "mcp-runtime"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("up output missing %q: %s", want, stdout.String())
		}
	}
}

func TestModelConnectStoresKeyAndUsesInferredName(t *testing.T) {
	var gotSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/model-providers":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "deepseek", "adapter": "openai_compatible",
				"base_url": "https://api.deepseek.com", "default_model": "deepseek-chat",
				"secret_ref": "deepseek_api_key", "secret_configured": true,
				"enabled": true, "default": true,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/admin/model-providers/deepseek/secret":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotSecret = body["api_key"]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "deepseek", "adapter": "openai_compatible",
				"base_url": "https://api.deepseek.com", "default_model": "deepseek-chat",
				"secret_ref": "deepseek_api_key", "secret_configured": true,
				"enabled": true, "default": true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/model-providers/deepseek/check":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "deepseek", "adapter": "openai_compatible",
				"base_url": "https://api.deepseek.com", "default_model": "deepseek-chat",
				"secret_ref": "deepseek_api_key", "secret_configured": true,
				"enabled": true, "default": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{
		"model", "connect",
		"--server", server.URL,
		"--admin-token", "admin-token",
		"--url", "api.deepseek.com",
		"--key", "sk-live",
		"--model", "deepseek-chat",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("model connect = %d, stderr = %s", code, stderr.String())
	}
	if gotSecret != "sk-live" {
		t.Fatalf("secret endpoint got %q", gotSecret)
	}
	if strings.Contains(stdout.String(), "sk-live") {
		t.Fatalf("stdout leaked key: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "model connected: deepseek") {
		t.Fatalf("output = %s", stdout.String())
	}
}
