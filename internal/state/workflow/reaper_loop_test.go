// Levier — internal/state/workflow
//
// Periodic reaper loop tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeReapStore struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeReapStore) ReapExpired(context.Context, time.Time, int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return 2, nil
	}
	return 0, nil
}

func (f *fakeReapStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestRunReaperRunsImmediatelyAndOnInterval(t *testing.T) {
	store := &fakeReapStore{}
	results := make(chan ReapResult, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunReaper(ctx, store, 10*time.Millisecond, 100, func(result ReapResult) {
			results <- result
		})
	}()

	select {
	case result := <-results:
		if result.Err != nil || result.Count != 2 {
			t.Fatalf("first reap = %+v, want count=2", result)
		}
	case <-time.After(time.Second):
		t.Fatal("reaper did not run immediately")
	}

	select {
	case <-results:
	case <-time.After(time.Second):
		t.Fatal("reaper did not run on interval")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper did not stop on context cancel")
	}
	if store.callCount() < 2 {
		t.Fatalf("reap calls = %d, want >= 2", store.callCount())
	}
}
