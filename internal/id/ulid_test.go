// Levier — internal/id
//
// ULID generation and parsing tests.
//
// Copyright 2026 The Levier Authors.
package id

import (
	"testing"
	"time"
)

func TestNewIsParseableAndUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		value := New()
		if len(value) != 26 {
			t.Fatalf("len(New()) = %d, want 26", len(value))
		}
		if _, err := Parse(value); err != nil {
			t.Fatalf("Parse(%q) error = %v", value, err)
		}
		if _, ok := seen[value]; ok {
			t.Fatalf("duplicate ULID %q", value)
		}
		seen[value] = struct{}{}
	}
}

func TestNewAtPreservesTimestamp(t *testing.T) {
	want := time.Date(2026, 11, 3, 12, 30, 0, 0, time.UTC)
	value, err := NewAt(want)
	if err != nil {
		t.Fatalf("NewAt() error = %v", err)
	}

	got, err := Timestamp(value)
	if err != nil {
		t.Fatalf("Timestamp() error = %v", err)
	}
	if got.UnixMilli() != want.UnixMilli() {
		t.Fatalf("Timestamp() = %v, want %v", got, want)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	if _, err := Parse("not-a-ulid"); err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
}
