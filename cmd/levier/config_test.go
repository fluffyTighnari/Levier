package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levier.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunConfigCheck(t *testing.T) {
	path := writeConfig(t, `
config_version: 1
environment: development
http:
  admin_token: "admin-example-token"
grpc:
  daemon_tokens:
    daemon-local-1: "daemon-example-token"
database:
  url: "postgres://example-user:example-pass@localhost:5432/levier"
`)
	var stdout, stderr bytes.Buffer
	code := runConfigCheck([]string{"--config", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("config check = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status:      ok") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunConfigCheckProduction(t *testing.T) {
	path := writeConfig(t, `
config_version: 1
environment: development
http:
  admin_token: "admin-change-me"
grpc:
  daemon_tokens:
    daemon-local-1: "change-me"
database:
  url: "postgres://change-me:change-me@localhost:5432/levier"
`)
	var stdout, stderr bytes.Buffer
	code := runConfigCheck([]string{"--config", path, "--production"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("config check --production = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid production config") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunConfigPrintRedactsSecrets(t *testing.T) {
	path := writeConfig(t, `
config_version: 1
environment: development
http:
  admin_token: "admin-example-token"
grpc:
  daemon_tokens:
    daemon-local-1: "daemon-example-token"
database:
  url: "postgres://example-user:example-pass@localhost:5432/levier"
`)
	var stdout, stderr bytes.Buffer
	code := runConfigPrint([]string{"--config", path, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("config print = %d, stderr = %s", code, stderr.String())
	}
	output := stdout.String()
	for _, secret := range []string{"admin-example-token", "daemon-example-token", "example-pass"} {
		if strings.Contains(output, secret) {
			t.Fatalf("config print leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "***") {
		t.Fatalf("config print did not redact: %s", output)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode config print: %v", err)
	}
	if decoded["config_version"] != float64(1) {
		t.Fatalf("config_version = %v", decoded["config_version"])
	}
}

func TestRunConfigHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runConfig([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("config help = %d", code)
	}
	if !strings.Contains(stdout.String(), "config check") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunConfigUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runConfig([]string{"nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("config nope = %d, want 2", code)
	}
}
