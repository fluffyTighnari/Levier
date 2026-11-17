package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactDatabaseURL(t *testing.T) {
	got := redactDatabaseURL("postgres://user:secret@localhost:5432/levier?sslmode=disable")
	if strings.Contains(got, "secret") {
		t.Fatalf("redacted URL still contains password: %s", got)
	}
	if !strings.Contains(got, "user:") || !strings.Contains(got, "localhost:5432/levier") {
		t.Fatalf("redacted URL = %s", got)
	}
}

func TestRunMigrateUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runMigrate([]string{"nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runMigrate(nope) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown migrate command") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}
