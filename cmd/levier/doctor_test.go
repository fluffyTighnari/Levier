package main

import (
	"os"
	"path/filepath"
	"testing"
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
