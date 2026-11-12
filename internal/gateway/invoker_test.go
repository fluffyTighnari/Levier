// Middleware — internal/gateway
//
// ToolInvoker routing tests.
//
// Copyright 2026 The Middleware Authors.
package gateway

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/register"
)

type fakeResolver struct {
	tools map[string]register.ToolMaterial
	res   map[string]register.Resolution
	err   error
}

func (f *fakeResolver) ResolveToolDecision(_ context.Context, _ string, name string) (register.ToolMaterial, register.Resolution, error) {
	if f.err != nil {
		return register.ToolMaterial{}, register.ResolutionNotFound, f.err
	}
	tool, ok := f.tools[name]
	if !ok {
		return register.ToolMaterial{}, register.ResolutionNotFound, nil
	}
	resolution := f.res[name]
	if resolution == "" {
		resolution = register.ResolutionFound
	}
	return tool, resolution, nil
}

type fakeLocal struct {
	result  connection.LocalToolResult
	err     error
	request connection.LocalToolRequest
}

func (f *fakeLocal) CallLocalTool(_ context.Context, _ string, request connection.LocalToolRequest) (connection.LocalToolResult, error) {
	f.request = request
	return f.result, f.err
}

type fakeGlobal struct {
	result GlobalResult
	err    error
	call   GlobalCall
}

func (f *fakeGlobal) Execute(_ context.Context, call GlobalCall) (GlobalResult, error) {
	f.call = call
	return f.result, f.err
}

func request(name string) *runtimev1.ToolCallRequest {
	return requestWithID("inv-1", name)
}

func requestWithID(invocationID, name string) *runtimev1.ToolCallRequest {
	return &runtimev1.ToolCallRequest{
		InvocationId: invocationID, ToolName: name, ArgumentsJson: `{"q":"x"}`,
	}
}

func TestInvokeLocalSuccess(t *testing.T) {
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"search": {Name: "search", Scope: "local", TimeoutMS: 1234},
	}}
	local := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "3"}}
	invoker := NewInvoker(resolver, local, nil)

	req := requestWithID("inv-local", "search")
	result := invoker.Invoke(context.Background(), "run-1", "app-1", req)
	if !result.GetOk() || result.GetResultJson() != "3" {
		t.Fatalf("result = %+v", result)
	}
	if local.request.InvocationID != req.GetInvocationId() || local.request.TimeoutMS != 1234 {
		t.Fatalf("request = %+v", local.request)
	}
}

func TestInvokeLocalErrorMappings(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		result    connection.LocalToolResult
		wantCode  string
		retryable bool
	}{
		{"unreachable", connection.ErrAppUnreachable, connection.LocalToolResult{}, "APP_UNREACHABLE", true},
		{"timeout", connection.ErrLocalToolTimeout, connection.LocalToolResult{}, "TOOL_TIMEOUT", true},
		{"result error", nil, connection.LocalToolResult{Error: &connection.LocalToolError{Code: "TOOL_DENIED"}}, "TOOL_DENIED", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
				"search": {Name: "search", Scope: "local", TimeoutMS: 1000},
			}}
			local := &fakeLocal{err: tt.err, result: tt.result}
			result := NewInvoker(resolver, local, nil).Invoke(context.Background(), "run-1", "app-1", request("search"))
			if result.GetOk() || result.GetError().GetCode() != tt.wantCode || result.GetError().GetRetryable() != tt.retryable {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestInvokeGlobalSuccessAndTimeout(t *testing.T) {
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"fetch": {Name: "fetch", Scope: "global", ExecutorRef: "builtin:http", TimeoutMS: 1000},
	}}
	global := &fakeGlobal{result: GlobalResult{OK: true, ResultJSON: "ok"}}
	invoker := NewInvoker(resolver, nil, NewExecutorRegistry(map[string]GlobalExecutor{
		"builtin:http": global,
	}))
	result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-global-1", "fetch"))
	if !result.GetOk() || global.call.ExecutorRef != "builtin:http" {
		t.Fatalf("result = %+v call=%+v", result, global.call)
	}

	global.err = context.DeadlineExceeded
	result = invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-global-2", "fetch"))
	if result.GetError().GetCode() != "TOOL_TIMEOUT" || !result.GetError().GetRetryable() {
		t.Fatalf("timeout result = %+v", result)
	}
}

func TestInvokeUnknownDeniedAndMCP(t *testing.T) {
	resolver := &fakeResolver{
		tools: map[string]register.ToolMaterial{
			"private":   {Name: "private", Scope: "global"},
			"mcp__x__y": {Name: "mcp__x__y", Scope: "local"},
		},
		res: map[string]register.Resolution{
			"private": register.ResolutionDenied,
		},
	}
	invoker := NewInvoker(resolver, &fakeLocal{}, nil)

	if result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-missing", "missing")); result.GetError().GetCode() != "TOOL_NOT_FOUND" {
		t.Fatalf("missing = %+v", result)
	}
	if result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-private", "private")); result.GetError().GetCode() != "TOOL_DENIED" {
		t.Fatalf("denied = %+v", result)
	}
	if result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-mcp", "mcp__x__y")); result.GetError().GetCode() != "TOOL_NOT_FOUND" {
		t.Fatalf("mcp = %+v", result)
	}
}

func TestInvokeInvalidAndResolverFailure(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	invoker := NewInvoker(resolver, nil, nil)
	if result := invoker.Invoke(context.Background(), "run-1", "app-1", nil); result.GetError().GetCode() != "BAD_REQUEST" {
		t.Fatalf("nil request = %+v", result)
	}
	if result := invoker.Invoke(context.Background(), "run-1", "app-1", request("x")); result.GetError().GetCode() != "TOOL_EXEC_ERROR" {
		t.Fatalf("resolver error = %+v", result)
	}
	_ = time.Second
}

func TestInvokerRecordsMetrics(t *testing.T) {
	registry := observe.NewRegistry()
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"search": {Name: "search", Scope: "local", TimeoutMS: 1000},
	}}
	local := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "3"}}
	invoker := NewInvoker(resolver, local, nil, WithMetrics(registry))
	if result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-metrics", "search")); !result.GetOk() {
		t.Fatalf("result = %+v", result)
	}

	var buf bytes.Buffer
	if err := registry.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		`mw_tool_invocations_total{scope="local",status="ok",tool="search"} 1`,
		`mw_tool_duration_seconds_count{scope="local",tool="search"} 1`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("metrics do not contain %q:\n%s", want, output)
		}
	}
}

type panicLocal struct{}

func (f *panicLocal) CallLocalTool(context.Context, string, connection.LocalToolRequest) (connection.LocalToolResult, error) {
	panic("tool boom")
}

func TestInvokerRecoversToolPanicAndClearsFlight(t *testing.T) {
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"bad": {Name: "bad", Scope: "local", TimeoutMS: 1000},
	}}
	invoker := NewInvoker(resolver, &panicLocal{}, nil)
	result := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-panic", "bad"))
	if result.GetError().GetCode() != "TOOL_EXEC_ERROR" {
		t.Fatalf("result = %+v", result)
	}
	invoker.state.mu.Lock()
	flights := len(invoker.state.flights)
	invoker.state.mu.Unlock()
	if flights != 0 {
		t.Fatalf("flights = %d, want 0", flights)
	}

	second := invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-panic", "bad"))
	if second.GetError().GetCode() != "TOOL_EXEC_ERROR" {
		t.Fatalf("second result = %+v", second)
	}
}

func TestInvokerCancelledExecutionAndWaiter(t *testing.T) {
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"search": {Name: "search", Scope: "local", TimeoutMS: 1000},
	}}
	local := &fakeLocal{err: context.Canceled}
	result := NewInvoker(resolver, local, nil).Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-cancel", "search"))
	if result.GetError().GetCode() != "RUN_CANCELLED" || result.GetError().GetRetryable() {
		t.Fatalf("result = %+v", result)
	}

	blocking := newBlockingLocal()
	invoker := NewInvoker(resolver, blocking, nil)
	done := make(chan *runtimev1.ToolCallResult, 1)
	go func() {
		done <- invoker.Invoke(context.Background(), "run-1", "app-1", requestWithID("inv-wait", "search"))
	}()
	<-blocking.started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waiter := invoker.Invoke(ctx, "run-1", "app-1", requestWithID("inv-wait", "search"))
	if waiter.GetError().GetCode() != "RUN_CANCELLED" || waiter.GetError().GetRetryable() {
		t.Fatalf("waiter = %+v", waiter)
	}
	close(blocking.release)
	owner := <-done
	if !owner.GetOk() {
		t.Fatalf("owner = %+v", owner)
	}
}

func TestInvokerClampsToolTimeout(t *testing.T) {
	resolver := &fakeResolver{tools: map[string]register.ToolMaterial{
		"search": {Name: "search", Scope: "local", TimeoutMS: 1000},
	}}
	local := &fakeLocal{result: connection.LocalToolResult{OK: true, ResultJSON: "ok"}}
	req := requestWithID("inv-clamp", "search")
	req.TimeoutMs = int64(24 * time.Hour / time.Millisecond)
	if result := NewInvoker(resolver, local, nil).Invoke(context.Background(), "run-1", "app-1", req); !result.GetOk() {
		t.Fatalf("result = %+v", result)
	}
	if local.request.TimeoutMS != maxToolTimeout.Milliseconds() {
		t.Fatalf("timeout = %d, want %d", local.request.TimeoutMS, maxToolTimeout.Milliseconds())
	}
}
