// Levier — internal/workflow
//
// Backoff helper tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		base    time.Duration
		max     time.Duration
		want    time.Duration
	}{
		{1, 2 * time.Second, 5 * time.Minute, 2 * time.Second},
		{2, 2 * time.Second, 5 * time.Minute, 4 * time.Second},
		{3, 2 * time.Second, 5 * time.Minute, 8 * time.Second},
		{0, 2 * time.Second, 5 * time.Minute, 2 * time.Second},
		{5, time.Second, 6 * time.Second, 6 * time.Second},
		{100, time.Second, 5 * time.Minute, 5 * time.Minute},
		{1, 0, 0, 2 * time.Second},
	}
	for _, tt := range tests {
		if got := Backoff(tt.attempt, tt.base, tt.max); got != tt.want {
			t.Fatalf("Backoff(%d,%v,%v) = %v, want %v", tt.attempt, tt.base, tt.max, got, tt.want)
		}
	}
}
