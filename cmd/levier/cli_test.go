package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunCLIVersionHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(version) = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"levier ", "commit:", "built:", "go:", "platform:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("version output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunCLIVersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"version", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(version --json) = %d, stderr = %s", code, stderr.String())
	}
	var info versionInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version JSON: %v (%s)", err, stdout.String())
	}
	if info.Version == "" || info.Go == "" || info.Platform == "" {
		t.Fatalf("incomplete version info: %+v", info)
	}
}

func TestRunCLIUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runCLI(nope) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: nope") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunCLIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(help) = %d", code)
	}
	if !strings.Contains(stdout.String(), "levier serve") {
		t.Fatalf("usage = %s", stdout.String())
	}
}
