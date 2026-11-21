package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStackFileUsesProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_STACK_FILE", "")
	compose := filepath.Join(home, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveProfile(cliProfile{StackFile: compose}); err != nil {
		t.Fatal(err)
	}
	got, err := resolveStackFile("")
	if err != nil {
		t.Fatalf("resolveStackFile() error = %v", err)
	}
	if got != compose {
		t.Fatalf("resolveStackFile() = %q, want %q", got, compose)
	}
}

func TestResolveStackTargetUsesComposeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_STACK_FILE", "")
	compose := filepath.Join(home, "deploy", "compose.yml")
	if err := os.MkdirAll(filepath.Dir(compose), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveProfile(cliProfile{StackFile: compose}); err != nil {
		t.Fatal(err)
	}
	target, err := resolveStackTarget("", "", "")
	if err != nil {
		t.Fatalf("resolveStackTarget() error = %v", err)
	}
	if target.projectDir != filepath.Dir(compose) {
		t.Fatalf("projectDir = %q, want %q", target.projectDir, filepath.Dir(compose))
	}
}

func TestSetupDetectsInstalledComposeBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_SERVER", "")
	t.Setenv("LEVIER_CREDENTIALS", "")
	t.Setenv("LEVIER_STACK_FILE", "")
	stackDir := filepath.Join(home, "stack")
	if err := os.MkdirAll(stackDir, 0o700); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(stackDir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"setup", "--server", "http://127.0.0.1:8080"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setup = %d, stderr = %s", code, stderr.String())
	}
	if loadProfile().StackFile != compose {
		t.Fatalf("stack file = %q, want %q", loadProfile().StackFile, compose)
	}
}

func TestStackUpDryRunFromOutsideRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEVIER_HOME", home)
	t.Setenv("LEVIER_STACK_FILE", "")
	compose := filepath.Join(home, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveProfile(cliProfile{StackFile: compose}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runStack([]string{"up", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("stack up --dry-run = %d, stderr = %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "docker compose") || !strings.Contains(output, "up") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, filepath.Dir(compose)) && !strings.Contains(output, home) {
		t.Fatalf("output does not use profile project directory: %s", output)
	}
}
