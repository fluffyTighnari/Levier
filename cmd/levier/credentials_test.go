package main

import (
	"os"
	"path/filepath"
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
