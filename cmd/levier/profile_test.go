package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupPersistsClientProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	t.Setenv("LEVIER_CONFIG", "")

	credentials := filepath.Join(home, "credentials", "demo.json")
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{
		"setup",
		"--server", "http://levier.internal:8080/",
		"--credentials", credentials,
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setup = %d, stderr = %s", code, stderr.String())
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode setup output: %v (%s)", err, stdout.String())
	}
	if result["saved"] != true {
		t.Fatalf("setup saved = %v", result["saved"])
	}
	path := filepath.Join(home, "config.yaml")
	if result["profile"] != path {
		t.Fatalf("profile = %v, want %s", result["profile"], path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if !bytes.Contains(data, []byte("http://levier.internal:8080")) {
		t.Fatalf("profile missing server: %s", data)
	}
	if defaultServerValue() != "http://levier.internal:8080" {
		t.Fatalf("defaultServerValue() = %q", defaultServerValue())
	}
	if defaultCredentialsValue() != credentials {
		t.Fatalf("defaultCredentialsValue() = %q, want %s", defaultCredentialsValue(), credentials)
	}
}

func TestDefaultCredentialsHonorsLevierHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_CREDENTIALS", "")
	path, err := defaultCredentialsPath("demo")
	if err != nil {
		t.Fatalf("defaultCredentialsPath() error = %v", err)
	}
	if path != filepath.Join(home, "demo.json") {
		t.Fatalf("path = %q, want %q", path, filepath.Join(home, "demo.json"))
	}
}

func TestDefaultCredentialsPathUsesPerAppName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_CREDENTIALS", "")
	if _, err := saveProfile(cliProfile{Credentials: filepath.Join(home, "old-app.json")}); err != nil {
		t.Fatal(err)
	}
	path, err := defaultCredentialsPath("new-app")
	if err != nil {
		t.Fatalf("defaultCredentialsPath() error = %v", err)
	}
	if path != filepath.Join(home, "new-app.json") {
		t.Fatalf("path = %q, want %q", path, filepath.Join(home, "new-app.json"))
	}
	oldPath, err := defaultCredentialsPath("old-app")
	if err != nil {
		t.Fatal(err)
	}
	if oldPath != filepath.Join(home, "old-app.json") {
		t.Fatalf("old path = %q", oldPath)
	}
}

func TestSetupShowDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"setup", "--show", "--server", "http://example.test:8080"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setup --show = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("setup --show wrote a profile: %v", err)
	}
}

func TestSetupPreservesExistingValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	t.Setenv("LEVIER_CONFIG", "")

	if _, err := saveProfile(cliProfile{
		Server:      "http://old.test:8080",
		Credentials: filepath.Join(home, "old.json"),
		MCPSocket:   filepath.Join(home, "mcp.sock"),
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"setup", "--server", "http://new.test:8080"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setup = %d, stderr = %s", code, stderr.String())
	}
	profile := loadProfile()
	if profile.Server != "http://new.test:8080" {
		t.Fatalf("server = %q", profile.Server)
	}
	if profile.Credentials != filepath.Join(home, "old.json") {
		t.Fatalf("credentials = %q", profile.Credentials)
	}
	if profile.MCPSocket != filepath.Join(home, "mcp.sock") {
		t.Fatalf("mcp socket = %q", profile.MCPSocket)
	}
}
