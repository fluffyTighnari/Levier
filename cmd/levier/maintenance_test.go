package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fluffyTighnari/levier/internal/config"
)

func TestRunMaintenanceHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMaintenance([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("maintenance help = %d", code)
	}
	for _, want := range []string{"maintenance status", "maintenance prune", "--yes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("maintenance help missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunMaintenanceUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMaintenance([]string{"nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("maintenance nope = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown maintenance command") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestMaintenancePruneRejectsConflictingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runMaintenancePrune([]string{"--yes", "--dry-run"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("prune --yes --dry-run = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not both") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestMaintenancePruneRefusesDisabledRetention(t *testing.T) {
	path := writeConfig(t, `
config_version: 1
environment: development
database:
  url: "postgres://example-user:example-pass@127.0.0.1:1/levier"
retention:
  enabled: false
  run_days: 30
  session_days: 7
  batch_size: 100
`)
	var stdout, stderr bytes.Buffer
	code := runMaintenancePrune([]string{"--config", path, "--yes"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("prune --yes with disabled retention = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "retention.enabled is false") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRetentionPolicy(t *testing.T) {
	cfg := config.Default()
	cfg.Retention.Enabled = true
	cfg.Retention.RunDays = 12
	cfg.Retention.SessionDays = 3
	cfg.Retention.BatchSize = 25
	policy := retentionPolicy(cfg)
	if !policy.Enabled || policy.RunDays != 12 || policy.SessionDays != 3 || policy.BatchSize != 25 {
		t.Fatalf("policy = %+v", policy)
	}
}
