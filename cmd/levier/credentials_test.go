package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteCredentialsRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if _, err := writeCredentials(path, appCredentials{AppID: "app_1", APIKey: "one"}, false); err != nil {
		t.Fatalf("writeCredentials() error = %v", err)
	}
	if _, err := writeCredentials(path, appCredentials{AppID: "app_1", APIKey: "two"}, false); err == nil {
		t.Fatal("writeCredentials() overwrite error = nil, want error")
	}
}

func TestWriteCredentialsBacksUpOnForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if _, err := writeCredentials(path, appCredentials{AppID: "app_1", APIKey: "one"}, false); err != nil {
		t.Fatalf("writeCredentials() error = %v", err)
	}
	backup, err := writeCredentials(path, appCredentials{AppID: "app_1", APIKey: "two"}, true)
	if err != nil {
		t.Fatalf("writeCredentials(force) error = %v", err)
	}
	if backup == "" {
		t.Fatal("backup path is empty")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	credentials, err := readCredentials(path)
	if err != nil {
		t.Fatalf("readCredentials() error = %v", err)
	}
	if credentials.APIKey != "two" {
		t.Fatalf("api key = %q, want two", credentials.APIKey)
	}
}

func TestCredentialsVersionServerAndFingerprint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	credentials := appCredentials{
		AppID:   "app_1",
		AppName: "demo",
		APIKey:  "secret-key",
		Server:  "http://127.0.0.1:8080/",
	}
	if _, err := writeCredentials(path, credentials, false); err != nil {
		t.Fatalf("writeCredentials() error = %v", err)
	}
	loaded, err := readCredentials(path)
	if err != nil {
		t.Fatalf("readCredentials() error = %v", err)
	}
	if loaded.CredentialVersion != credentialVersion {
		t.Fatalf("CredentialVersion = %d", loaded.CredentialVersion)
	}
	if loaded.Server != "http://127.0.0.1:8080" {
		t.Fatalf("Server = %q", loaded.Server)
	}
	if loaded.CreatedAt == "" || loaded.UpdatedAt == "" {
		t.Fatalf("timestamps missing: %+v", loaded)
	}
	if fingerprint := credentialFingerprint("secret-key"); len(fingerprint) != 12 {
		t.Fatalf("fingerprint = %q", fingerprint)
	}
}

func TestReadCredentialsRejectsLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	data := []byte(`{"app_id":"app_1","app_name":"demo","api_key":"secret-key"}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if _, err := readCredentials(path); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("readCredentials() error = %v", err)
	}
}

func TestReadCredentialsRejectsFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	data := []byte(`{"credential_version":2,"app_id":"app_1","app_name":"demo","api_key":"secret-key"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if _, err := readCredentials(path); err == nil || !strings.Contains(err.Error(), "unsupported credentials version") {
		t.Fatalf("readCredentials() error = %v", err)
	}
}

func TestSafeAppName(t *testing.T) {
	for input, want := range map[string]string{
		"counter-agent": "counter-agent",
		"my app":        "my-app",
		"../bad":        "bad",
		"":              "app",
	} {
		if got := safeAppName(input); got != want {
			t.Fatalf("safeAppName(%q) = %q, want %q", input, got, want)
		}
	}
	if strings.Contains(safeAppName("a/b"), "/") {
		t.Fatal("safeAppName left a path separator")
	}
}
