// Middleware — internal/clock
//
// Clock behavior tests.
//
// Copyright 2026 The Middleware Authors.
package clock

import (
	"testing"
	"time"
)

func TestFakeAdvanceAndSet(t *testing.T) {
	start := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	fake := NewFake(start)

	if got := fake.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}

	fake.Advance(90 * time.Second)
	if got, want := fake.Now(), start.Add(90*time.Second); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}

	replacement := start.Add(24 * time.Hour)
	fake.Set(replacement)
	if got := fake.Now(); !got.Equal(replacement) {
		t.Fatalf("Now() = %v, want %v", got, replacement)
	}
}

func TestSystemReturnsUTC(t *testing.T) {
	if got := (System{}).Now(); got.Location() != time.UTC {
		t.Fatalf("System.Now() location = %v, want UTC", got.Location())
	}
}
