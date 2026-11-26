// Levier — internal/state/workflow
//
// Error classification tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	cause := errors.New("temporary")
	err := RetryAfter(cause, 2*time.Second)

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(err, cause) = false")
	}
	if IsPermanent(err) {
		t.Fatal("IsPermanent(retryable) = true")
	}
	delay, ok := RetryAfterDuration(err)
	if !ok || delay != 2*time.Second {
		t.Fatalf("RetryAfterDuration() = (%v,%v)", delay, ok)
	}
	if RetryAfter(nil, time.Second) != nil {
		t.Fatal("RetryAfter(nil) != nil")
	}
	if _, ok := RetryAfterDuration(cause); ok {
		t.Fatal("plain error has retry delay")
	}
}

func TestPermanent(t *testing.T) {
	cause := errors.New("bad input")
	err := Permanent(cause)

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(err, cause) = false")
	}
	if !IsPermanent(err) {
		t.Fatal("IsPermanent(permanent) = false")
	}
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) != nil")
	}
}

func TestHandlerFunc(t *testing.T) {
	called := false
	handler := HandlerFunc(func(ctx context.Context, task Task, lease Lease) error {
		called = true
		return nil
	})
	if err := handler.Handle(context.Background(), Task{}, Lease{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}
