package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluffyTighnari/levier/internal/selfdesc"
)

func TestReadAppCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	body := `{"app_id":"app_1","app_name":"demo","api_key":"mw_test"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	app, err := readCredentials(path)
	if err != nil {
		t.Fatalf("readCredentials() error = %v", err)
	}
	if app.AppID != "app_1" || app.APIKey != "mw_test" {
		t.Fatalf("app = %+v", app)
	}
}

func TestReadAppCredentialsRejectsIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"app_id":"app_1"}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if _, err := readCredentials(path); err == nil {
		t.Fatal("readCredentials() error = nil, want error")
	}
}

func TestCheckMeta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(selfdesc.New(selfdesc.Options{Version: "v-test"}))
	}))
	defer server.Close()
	check := checkMeta(context.Background(), server.Client(), server.URL)
	if check.Status != "ok" {
		t.Fatalf("checkMeta = %+v", check)
	}
}

func TestCheckMetaRejectsMissingEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	check := checkMeta(context.Background(), server.Client(), server.URL)
	if check.Status != "fail" {
		t.Fatalf("checkMeta = %+v", check)
	}
}

func TestCheckMetaRejectsUnsupportedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_version": 2,
			"service":      "levier",
		})
	}))
	defer server.Close()
	check := checkMeta(context.Background(), server.Client(), server.URL)
	if check.Status != "fail" || check.Message == "" {
		t.Fatalf("checkMeta = %+v", check)
	}
}
