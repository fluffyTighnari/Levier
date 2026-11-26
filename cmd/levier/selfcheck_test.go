// Levier — selfcheck CLI tests.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/fluffyTighnari/levier/internal/ops/selfcheck"
)

func TestRunSelfcheckJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runSelfcheck([]string{
		"--config", filepath.Join(t.TempDir(), "missing.yaml"),
		"--server", "http://127.0.0.1:1",
		"--timeout", "50ms",
		"--json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("selfcheck = %d, want 1, stderr = %s", code, stderr.String())
	}
	var report selfcheck.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode selfcheck JSON: %v (%s)", err, stdout.String())
	}
	if len(report.Hops) < 10 || report.Hops[0].Name != "config" {
		t.Fatalf("report = %+v", report)
	}
}
