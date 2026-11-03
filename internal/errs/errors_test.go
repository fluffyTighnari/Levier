// Middleware — internal/errs
//
// Error model tests.
//
// Copyright 2026 The Middleware Authors.
package errs

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorFormattingAndUnwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := Wrap(CodeAppUnreachable, "app ws disconnected", cause)

	if !strings.Contains(err.Error(), "APP_UNREACHABLE") {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(err, cause) = false")
	}
	if !IsRetryable(err) {
		t.Fatal("IsRetryable(err) = false")
	}
	if CodeOf(err) != CodeAppUnreachable {
		t.Fatalf("CodeOf(err) = %q", CodeOf(err))
	}
}

func TestRetryabilityTable(t *testing.T) {
	tests := []struct {
		code Code
		want bool
	}{
		{CodeNoDaemonAvailable, true},
		{CodeToolTimeout, true},
		{CodeQueueUnavailable, true},
		{CodeAppUnreachable, true},
		{CodeToolNotFound, false},
		{CodeToolDenied, false},
		{CodeRunCancelled, false},
		{CodeAuthFailed, false},
		{CodeInternal, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := New(tt.code, "x").Retryable; got != tt.want {
				t.Fatalf("retryable = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		code Code
		want int
	}{
		{CodeBadEnvelope, 400},
		{CodeAuthFailed, 401},
		{CodeToolDenied, 403},
		{CodeToolNotFound, 404},
		{CodeRunCancelled, 409},
		{CodeQueueOverflow, 429},
		{CodeNoDaemonAvailable, 503},
		{CodeToolTimeout, 504},
		{CodeInternal, 500},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := HTTPStatus(tt.code); got != tt.want {
				t.Fatalf("HTTPStatus(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestNilAndUnknownErrors(t *testing.T) {
	if IsRetryable(nil) {
		t.Fatal("IsRetryable(nil) = true")
	}
	if CodeOf(nil) != "" {
		t.Fatalf("CodeOf(nil) = %q", CodeOf(nil))
	}
	if HTTPStatusOf(errors.New("plain")) != 500 {
		t.Fatal("HTTPStatusOf(plain) != 500")
	}
}
