// Levier — internal/observe
//
// Structured logging tests.
//
// Copyright 2026 The Levier Authors.
package observe

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewLoggerJSONFields(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLoggerWithWriter("info", "json", &buf)
	if err != nil {
		t.Fatalf("NewLoggerWithWriter() error = %v", err)
	}

	logger.Info("run accepted", FieldRunID, "run-1", FieldAttempt, 2)
	output := buf.String()
	for _, want := range []string{`"run_id":"run-1"`, `"attempt":2`, `"msg":"run accepted"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestNewLoggerRejectsBadConfig(t *testing.T) {
	if _, err := NewLoggerWithWriter("verbose", "json", &bytes.Buffer{}); err == nil {
		t.Fatal("expected bad level error")
	}
	if _, err := NewLoggerWithWriter("info", "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected bad format error")
	}
	if _, err := NewLoggerWithWriter("info", "json", nil); err == nil {
		t.Fatal("expected nil writer error")
	}
}
