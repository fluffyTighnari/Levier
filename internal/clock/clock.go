// Package clock provides an injectable clock for time-dependent behavior.
//
// Levier — internal/clock
// Copyright 2026 The Levier Authors.
package clock

import (
	"sync"
	"time"
)

// Clock is the minimal time source used by middleware packages.
type Clock interface {
	Now() time.Time
}

// System uses the host clock and always returns UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time {
	return time.Now().UTC()
}

// Fake is a manually advanced clock for tests.
type Fake struct {
	mu  sync.RWMutex
	now time.Time
}

// NewFake creates a fake clock pinned to t.
func NewFake(t time.Time) *Fake {
	return &Fake{now: t.UTC()}
}

// Now returns the fake clock's current UTC time.
func (f *Fake) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set replaces the fake clock's current UTC time.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}
