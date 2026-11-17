// Levier — internal/observe
//
// Structured logging and canonical field names.
//
// Copyright 2026 The Levier Authors.
package observe

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	FieldAppID        = "app_id"
	FieldRunID        = "run_id"
	FieldTaskID       = "task_id"
	FieldInvocationID = "invocation_id"
	FieldTraceID      = "trace_id"
	FieldDaemonID     = "daemon_id"
	FieldAttempt      = "attempt"
	FieldLeaseEpoch   = "lease_epoch"
	FieldSessionID    = "session_id"
	FieldRequestID    = "request_id"
	FieldToolName     = "tool_name"
	FieldToolScope    = "tool_scope"
	FieldStatus       = "status"
	FieldErrorCode    = "error_code"
	FieldEventType    = "event_type"
	FieldReason       = "reason"
	FieldMethod       = "method"
	FieldRoute        = "route"
	FieldDurationMS   = "duration_ms"
	FieldRemoteAddr   = "remote_addr"
)

// NewLogger creates a structured logger writing to stdout.
func NewLogger(level, format string) (*slog.Logger, error) {
	return NewLoggerWithWriter(level, format, os.Stdout)
}

// NewLoggerWithWriter creates a structured logger writing to w.
func NewLoggerWithWriter(level, format string, w io.Writer) (*slog.Logger, error) {
	if w == nil {
		return nil, fmt.Errorf("log writer is nil")
	}

	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	case "text":
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	return slog.New(handler), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}
