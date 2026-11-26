// Levier — internal/runtime/gateway
//
// In-process single-flight and terminal-result cache for tool invocations.
//
// Copyright 2026 The Levier Authors.
package gateway

import (
	"context"
	"sync"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

const (
	defaultResultCacheTTL = 5 * time.Minute
	defaultResultCacheMax = 1024
)

// flight is one in-progress invocation shared by duplicate callers.
type flight struct {
	done   chan struct{}
	result *runtimev1.ToolCallResult
}

// cacheEntry is one terminal result with an expiry.
type cacheEntry struct {
	result    *runtimev1.ToolCallResult
	expiresAt time.Time
}

// invocationState deduplicates concurrent invocations and caches terminal results.
type invocationState struct {
	mu       sync.Mutex
	flights  map[string]*flight
	cache    map[string]cacheEntry
	cacheTTL time.Duration
	maxCache int
	now      func() time.Time
}

func newInvocationState(ttl time.Duration, maxEntries int) *invocationState {
	if ttl <= 0 {
		ttl = defaultResultCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultResultCacheMax
	}
	return &invocationState{
		flights:  make(map[string]*flight),
		cache:    make(map[string]cacheEntry),
		cacheTTL: ttl,
		maxCache: maxEntries,
		now:      time.Now,
	}
}

// get returns a cached terminal result when it has not expired.
func (s *invocationState) get(invocationID string) (*runtimev1.ToolCallResult, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[invocationID]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.IsZero() && s.now().After(entry.expiresAt) {
		delete(s.cache, invocationID)
		return nil, false
	}
	return cloneResult(entry.result), true
}

// put caches one terminal result and drops expired entries.
func (s *invocationState) put(invocationID string, result *runtimev1.ToolCallResult) {
	if s == nil || invocationID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, entry := range s.cache {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(s.cache, key)
		}
	}
	if len(s.cache) >= s.maxCache {
		for key := range s.cache {
			delete(s.cache, key)
			break
		}
	}
	s.cache[invocationID] = cacheEntry{
		result:    cloneResult(result),
		expiresAt: now.Add(s.cacheTTL),
	}
}

// acquire returns the existing flight for an invocation or installs a new one.
func (s *invocationState) acquire(invocationID string) (*flight, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.flights[invocationID]; ok {
		return existing, false
	}
	created := &flight{done: make(chan struct{})}
	s.flights[invocationID] = created
	return created, true
}

// wait blocks until the owner publishes a result or the caller gives up.
func (s *invocationState) wait(ctx context.Context, f *flight) (*runtimev1.ToolCallResult, error) {
	select {
	case <-f.done:
		return cloneResult(f.result), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// finish publishes the owner result and removes the flight.
func (s *invocationState) finish(invocationID string, f *flight, result *runtimev1.ToolCallResult) {
	if s == nil || f == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.flights[invocationID]; ok && current == f {
		delete(s.flights, invocationID)
	}
	f.result = cloneResult(result)
	close(f.done)
}

// cloneResult copies a ToolCallResult so callers cannot mutate shared state.
func cloneResult(result *runtimev1.ToolCallResult) *runtimev1.ToolCallResult {
	if result == nil {
		return nil
	}
	cloned := &runtimev1.ToolCallResult{
		InvocationId: result.GetInvocationId(),
		Ok:           result.GetOk(),
		ResultJson:   result.GetResultJson(),
	}
	if result.GetError() != nil {
		cloned.Error = &runtimev1.ErrorInfo{
			Code:      result.GetError().GetCode(),
			Message:   result.GetError().GetMessage(),
			Retryable: result.GetError().GetRetryable(),
		}
	}
	return cloned
}
