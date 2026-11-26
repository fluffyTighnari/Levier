// Levier — internal/runtime/gateway
//
// Tool invocation idempotency and single-flight tests.
//
// Copyright 2026 The Levier Authors.
package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/state/register"
)

// storedInvocation is one fake store row.
type storedInvocation struct {
	status string
	result *runtimev1.ToolCallResult
}

// fakeInvocationStore implements InvocationStore in memory.
type fakeInvocationStore struct {
	mu   sync.Mutex
	rows map[string]*storedInvocation
}

func newFakeInvocationStore() *fakeInvocationStore {
	return &fakeInvocationStore{rows: make(map[string]*storedInvocation)}
}

func (f *fakeInvocationStore) Begin(_ context.Context, invocationID, _ string, _ string) (bool, *StoredResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row, ok := f.rows[invocationID]; ok {
		if row.status == "in_flight" {
			return false, nil, nil
		}
		return false, &StoredResult{Result: cloneResult(row.result)}, nil
	}
	f.rows[invocationID] = &storedInvocation{status: "in_flight"}
	return true, nil, nil
}

func (f *fakeInvocationStore) Get(_ context.Context, invocationID string) (*StoredResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[invocationID]
	if !ok || row.status == "in_flight" {
		return nil, false, nil
	}
	return &StoredResult{Result: cloneResult(row.result)}, true, nil
}

func (f *fakeInvocationStore) Complete(_ context.Context, invocationID string, result *runtimev1.ToolCallResult) error {
	return f.finish(invocationID, "completed", result)
}

func (f *fakeInvocationStore) Fail(_ context.Context, invocationID string, result *runtimev1.ToolCallResult) error {
	return f.finish(invocationID, "failed", result)
}

func (f *fakeInvocationStore) finish(invocationID, status string, result *runtimev1.ToolCallResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[invocationID] = &storedInvocation{status: status, result: cloneResult(result)}
	return nil
}

func (f *fakeInvocationStore) status(invocationID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row, ok := f.rows[invocationID]; ok {
		return row.status
	}
	return ""
}

func (f *fakeInvocationStore) putInFlight(invocationID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[invocationID] = &storedInvocation{status: "in_flight"}
}

// blockingLocal is a LocalToolCaller that blocks until released.
type blockingLocal struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingLocal() *blockingLocal {
	return &blockingLocal{started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingLocal) CallLocalTool(context.Context, string, connection.LocalToolRequest) (connection.LocalToolResult, error) {
	b.calls.Add(1)
	b.once.Do(func() { close(b.started) })
	<-b.release
	return connection.LocalToolResult{OK: true, ResultJSON: "ok"}, nil
}

func localToolResolver() *fakeResolver {
	return &fakeResolver{tools: map[string]register.ToolMaterial{
		"search": {Name: "search", Scope: "local", TimeoutMS: 5000},
	}}
}

func TestInvokeConcurrentSameInvocationRunsOnce(t *testing.T) {
	store := newFakeInvocationStore()
	local := newBlockingLocal()
	invoker := NewInvoker(localToolResolver(), local, nil, WithInvocationStore(store))

	const callers = 8
	results := make(chan *runtimev1.ToolCallResult, callers)
	var wg sync.WaitGroup
	for n := 0; n < callers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
		}()
	}
	<-local.started
	close(local.release)
	wg.Wait()
	close(results)

	if calls := local.calls.Load(); calls != 1 {
		t.Fatalf("local calls = %d, want 1", calls)
	}
	for result := range results {
		if !result.GetOk() || result.GetResultJson() != "ok" {
			t.Fatalf("result = %+v", result)
		}
	}
	if got := store.status("inv-1"); got != "completed" {
		t.Fatalf("store status = %q", got)
	}
}

func TestInvokeDuplicateUsesCachedResult(t *testing.T) {
	local := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "3"}}
	invoker := NewInvoker(localToolResolver(), local, nil)

	first := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	second := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if first.GetResultJson() != "3" || second.GetResultJson() != "3" {
		t.Fatalf("results = %+v / %+v", first, second)
	}
	// fakeLocal does not count calls, so assert the invocation did not route
	// again by mutating its result.
	local.result = connection.LocalToolResult{OK: true, ResultJSON: "changed"}
	third := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if third.GetResultJson() != "3" {
		t.Fatalf("cached result was bypassed: %+v", third)
	}
}

func TestInvokeUsesStoredResultAcrossInvokers(t *testing.T) {
	store := newFakeInvocationStore()
	firstLocal := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "persisted"}}
	first := NewInvoker(localToolResolver(), firstLocal, nil, WithInvocationStore(store))
	if result := first.Invoke(context.Background(), "run-1", "app-1", request("search")); !result.GetOk() {
		t.Fatalf("first result = %+v", result)
	}

	secondLocal := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "re-executed"}}
	second := NewInvoker(localToolResolver(), secondLocal, nil, WithInvocationStore(store))
	result := second.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if !result.GetOk() || result.GetResultJson() != "persisted" {
		t.Fatalf("second result = %+v", result)
	}
	if secondLocal.request.InvocationID != "" {
		t.Fatalf("second invoker executed the tool: %+v", secondLocal.request)
	}
}

func TestInvokeFailedResultIsPersisted(t *testing.T) {
	store := newFakeInvocationStore()
	local := &fakeLocal{result: connection.LocalToolResult{
		OK:    false,
		Error: &connection.LocalToolError{Code: "TOOL_DENIED", Message: "not allowed", Retryable: false},
	}}
	invoker := NewInvoker(localToolResolver(), local, nil, WithInvocationStore(store))

	first := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if first.GetOk() || first.GetError().GetCode() != "TOOL_DENIED" {
		t.Fatalf("first result = %+v", first)
	}
	if got := store.status("inv-1"); got != "failed" {
		t.Fatalf("store status = %q", got)
	}
	second := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if second.GetError().GetCode() != "TOOL_DENIED" || second.GetError().GetMessage() != "not allowed" {
		t.Fatalf("second result = %+v", second)
	}
}

func TestInvokeWaiterHonoursContext(t *testing.T) {
	local := newBlockingLocal()
	invoker := NewInvoker(localToolResolver(), local, nil)

	firstDone := make(chan *runtimev1.ToolCallResult, 1)
	go func() {
		firstDone <- invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	}()
	<-local.started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waiter := invoker.Invoke(ctx, "run-1", "app-1", request("search"))
	if waiter.GetError().GetCode() != "TOOL_TIMEOUT" || !waiter.GetError().GetRetryable() {
		t.Fatalf("waiter result = %+v", waiter)
	}
	close(local.release)
	if result := <-firstDone; !result.GetOk() {
		t.Fatalf("owner result = %+v", result)
	}
}

func TestInvokeBusyStoreReturnsRetryableError(t *testing.T) {
	store := newFakeInvocationStore()
	store.putInFlight("inv-1")
	local := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "should not run"}}
	invoker := NewInvoker(localToolResolver(), local, nil, WithInvocationStore(store))

	result := invoker.Invoke(context.Background(), "run-1", "app-1", request("search"))
	if result.GetOk() || !result.GetError().GetRetryable() {
		t.Fatalf("result = %+v", result)
	}
	if local.request.InvocationID != "" {
		t.Fatal("busy invocation was executed")
	}
}

func TestResultCacheExpires(t *testing.T) {
	state := newInvocationState(time.Minute, 2)
	now := time.Now()
	state.now = func() time.Time { return now }
	state.put("inv-1", &runtimev1.ToolCallResult{InvocationId: "inv-1", Ok: true, ResultJson: "1"})
	if _, ok := state.get("inv-1"); !ok {
		t.Fatal("fresh entry was not cached")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := state.get("inv-1"); ok {
		t.Fatal("expired entry was returned")
	}
}
