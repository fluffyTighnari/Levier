// Middleware — internal/gateway
//
// ToolInvoker resolves ToolDefs, deduplicates invocations, and routes
// local/global calls.
//
// Copyright 2026 The Middleware Authors.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"log/slog"
	"strings"
	"time"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/register"
)

const defaultToolTimeout = 60 * time.Second

// ToolResolver is the Registry subset used for routing.
type ToolResolver interface {
	ResolveToolDecision(ctx context.Context, appID, name string) (register.ToolMaterial, register.Resolution, error)
}

// LocalToolCaller executes local tools through AppHub.
type LocalToolCaller interface {
	CallLocalTool(ctx context.Context, appID string, request connection.LocalToolRequest) (connection.LocalToolResult, error)
}

// GlobalCall is one global tool invocation.
type GlobalCall struct {
	RunID          string
	AppID          string
	InvocationID   string
	ToolName       string
	ArgumentsJSON  string
	ExecutorRef    string
	ToolConfigJSON string
	Timeout        time.Duration
}

// GlobalResult is a global executor result.
type GlobalResult struct {
	OK         bool
	ResultJSON string
	Code       string
	Message    string
	Retryable  bool
}

// GlobalExecutor executes one registered global tool.
type GlobalExecutor interface {
	Execute(ctx context.Context, call GlobalCall) (GlobalResult, error)
}

// ExecutorRegistry maps executor_ref to a global executor.
type ExecutorRegistry struct {
	executors map[string]GlobalExecutor
}

// NewExecutorRegistry returns a global executor registry.
func NewExecutorRegistry(executors map[string]GlobalExecutor) *ExecutorRegistry {
	copied := make(map[string]GlobalExecutor, len(executors))
	for key, value := range executors {
		copied[key] = value
	}
	return &ExecutorRegistry{executors: copied}
}

// Resolve returns the executor for executor_ref.
func (r *ExecutorRegistry) Resolve(executorRef string) (GlobalExecutor, bool) {
	if r == nil {
		return nil, false
	}
	executor, ok := r.executors[executorRef]
	return executor, ok
}

// InvokerOption customizes an Invoker.
type InvokerOption func(*Invoker)

// WithInvocationStore enables PostgreSQL-backed idempotency.
func WithInvocationStore(store InvocationStore) InvokerOption {
	return func(invoker *Invoker) {
		invoker.store = store
	}
}

// WithMetrics attaches a metrics registry.
func WithMetrics(metrics *observe.Registry) InvokerOption {
	return func(invoker *Invoker) {
		invoker.metrics = metrics
	}
}

// WithResultCache overrides the in-process cache TTL and capacity.
func WithResultCache(ttl time.Duration, maxEntries int) InvokerOption {
	return func(invoker *Invoker) {
		invoker.state = newInvocationState(ttl, maxEntries)
	}
}

// Invoker routes one ToolCallRequest to a local or global executor.
type Invoker struct {
	resolver ToolResolver
	local    LocalToolCaller
	globals  *ExecutorRegistry
	store    InvocationStore
	state    *invocationState
	metrics  *observe.Registry
}

// NewInvoker returns a tool invoker.
func NewInvoker(resolver ToolResolver, local LocalToolCaller, globals *ExecutorRegistry, opts ...InvokerOption) *Invoker {
	invoker := &Invoker{
		resolver: resolver,
		local:    local,
		globals:  globals,
		state:    newInvocationState(defaultResultCacheTTL, defaultResultCacheMax),
	}
	for _, opt := range opts {
		opt(invoker)
	}
	return invoker
}

// Invoke resolves and executes one tool request exactly once per invocation_id.
func (i *Invoker) Invoke(
	ctx context.Context,
	runID string,
	appID string,
	request *runtimev1.ToolCallRequest,
) *runtimev1.ToolCallResult {
	if request == nil || request.GetInvocationId() == "" || request.GetToolName() == "" {
		return toolError("", string(errs.CodeBadRequest), "invocation_id and tool_name are required", false)
	}
	if i == nil || i.resolver == nil {
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), "tool registry is not configured", false)
	}
	invocationID := request.GetInvocationId()

	if cached, ok := i.state.get(invocationID); ok {
		return cached
	}
	pending, owner := i.state.acquire(invocationID)
	if !owner {
		result, err := i.state.wait(ctx, pending)
		if err != nil {
			return toolError(invocationID, string(errs.CodeToolTimeout), "cancelled while waiting for invocation", true)
		}
		return result
	}
	result := i.execute(ctx, runID, appID, request)
	i.state.finish(invocationID, pending, result)
	return cloneResult(result)
}

// execute claims the invocation in the durable store, then runs it.
func (i *Invoker) execute(
	ctx context.Context,
	runID, appID string,
	request *runtimev1.ToolCallRequest,
) *runtimev1.ToolCallResult {
	invocationID := request.GetInvocationId()
	if i.store != nil {
		owner, stored, err := i.store.Begin(ctx, invocationID, runID, request.GetToolName())
		if err != nil {
			return toolError(invocationID, string(errs.CodeToolExecError), err.Error(), false)
		}
		if !owner {
			if stored != nil && stored.Result != nil {
				i.state.put(invocationID, stored.Result)
				return stored.Result
			}
			return toolError(invocationID, string(errs.CodeToolExecError), "invocation is already in flight", true)
		}
	}

	result := i.invokeOnce(ctx, runID, appID, request)
	if i.store != nil {
		var err error
		if result.GetOk() {
			err = i.store.Complete(ctx, invocationID, result)
		} else {
			err = i.store.Fail(ctx, invocationID, result)
		}
		if err != nil {
			slog.Warn("gateway persist invocation failed", "run_id", runID, "invocation_id", invocationID, "err", err)
		}
	}
	i.state.put(invocationID, result)
	return result
}

// invokeOnce performs routing and execution without idempotency.
func (i *Invoker) invokeOnce(
	ctx context.Context,
	runID string,
	appID string,
	request *runtimev1.ToolCallRequest,
) *runtimev1.ToolCallResult {
	tool, resolution, err := i.resolver.ResolveToolDecision(ctx, appID, request.GetToolName())
	if err != nil {
		return toolError(request.GetInvocationId(), string(errs.CodeToolExecError), err.Error(), false)
	}
	switch resolution {
	case register.ResolutionDenied:
		return toolError(request.GetInvocationId(), string(errs.CodeToolDenied), "tool is not allowed for this app", false)
	case register.ResolutionNotFound:
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), "tool not found", false)
	case register.ResolutionFound:
	default:
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), "tool not found", false)
	}
	if strings.HasPrefix(tool.Name, "mcp__") {
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), "MCP tools are routed by the MCP runtime", false)
	}

	timeout := time.Duration(request.GetTimeoutMs()) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(tool.TimeoutMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = defaultToolTimeout
	}

	started := time.Now()
	var result *runtimev1.ToolCallResult
	switch tool.Scope {
	case "local":
		result = i.invokeLocal(ctx, runID, appID, request, timeout)
	case "global":
		result = i.invokeGlobal(ctx, runID, appID, request, tool, timeout)
	default:
		result = toolError(request.GetInvocationId(), string(errs.CodeToolExecError), fmt.Sprintf("unsupported tool scope %q", tool.Scope), false)
	}
	if i.metrics != nil {
		status := "ok"
		if !result.GetOk() {
			status = "error"
		}
		labels := map[string]string{"tool": tool.Name, "scope": tool.Scope}
		i.metrics.Inc(observe.MetricToolInvocationsTotal, "Tool executions by tool, scope, and status.",
			map[string]string{"tool": tool.Name, "scope": tool.Scope, "status": status})
		i.metrics.Observe(observe.MetricToolDuration, "Tool execution duration in seconds.",
			labels, time.Since(started).Seconds())
	}
	return result
}

func (i *Invoker) invokeLocal(
	ctx context.Context,
	runID, appID string,
	request *runtimev1.ToolCallRequest,
	timeout time.Duration,
) *runtimev1.ToolCallResult {
	if i.local == nil {
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotAvailable), "AppHub is not configured", false)
	}
	result, err := i.local.CallLocalTool(ctx, appID, connection.LocalToolRequest{
		InvocationID:  request.GetInvocationId(),
		RunID:         runID,
		ToolName:      request.GetToolName(),
		ArgumentsJSON: request.GetArgumentsJson(),
		TimeoutMS:     timeout.Milliseconds(),
	})
	if err != nil {
		switch {
		case errors.Is(err, connection.ErrAppUnreachable):
			return toolError(request.GetInvocationId(), string(errs.CodeAppUnreachable), err.Error(), true)
		case errors.Is(err, connection.ErrLocalToolTimeout), errors.Is(err, context.DeadlineExceeded):
			return toolError(request.GetInvocationId(), string(errs.CodeToolTimeout), "local tool timed out", true)
		default:
			return toolError(request.GetInvocationId(), string(errs.CodeToolExecError), err.Error(), false)
		}
	}
	if result.OK {
		return &runtimev1.ToolCallResult{
			InvocationId: request.GetInvocationId(),
			Ok:           true,
			ResultJson:   result.ResultJSON,
		}
	}
	code := string(errs.CodeToolExecError)
	message := "local tool failed"
	retryable := false
	if result.Error != nil {
		if result.Error.Code != "" {
			code = result.Error.Code
		}
		message = result.Error.Message
		retryable = result.Error.Retryable
	}
	return toolError(request.GetInvocationId(), code, message, retryable)
}

func (i *Invoker) invokeGlobal(
	ctx context.Context,
	runID, appID string,
	request *runtimev1.ToolCallRequest,
	tool register.ToolMaterial,
	timeout time.Duration,
) *runtimev1.ToolCallResult {
	if i.globals == nil {
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), "global executor registry is not configured", false)
	}
	executor, ok := i.globals.Resolve(tool.ExecutorRef)
	if !ok {
		return toolError(request.GetInvocationId(), string(errs.CodeToolNotFound), fmt.Sprintf("executor %q is not registered", tool.ExecutorRef), false)
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := executor.Execute(callCtx, GlobalCall{
		RunID:          runID,
		AppID:          appID,
		InvocationID:   request.GetInvocationId(),
		ToolName:       request.GetToolName(),
		ArgumentsJSON:  request.GetArgumentsJson(),
		ExecutorRef:    tool.ExecutorRef,
		ToolConfigJSON: tool.ConfigJSON,
		Timeout:        timeout,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return toolError(request.GetInvocationId(), string(errs.CodeToolTimeout), "global tool timed out", true)
		}
		return toolError(request.GetInvocationId(), string(errs.CodeToolExecError), err.Error(), false)
	}
	if result.OK {
		return &runtimev1.ToolCallResult{
			InvocationId: request.GetInvocationId(),
			Ok:           true,
			ResultJson:   result.ResultJSON,
		}
	}
	code := result.Code
	if code == "" {
		code = string(errs.CodeToolExecError)
	}
	message := result.Message
	if message == "" {
		message = "global tool failed"
	}
	return toolError(request.GetInvocationId(), code, message, result.Retryable)
}

func toolError(invocationID, code, message string, retryable bool) *runtimev1.ToolCallResult {
	return &runtimev1.ToolCallResult{
		InvocationId: invocationID,
		Ok:           false,
		Error: &runtimev1.ErrorInfo{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
}
