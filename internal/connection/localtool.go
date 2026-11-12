// Middleware — connection
//
// Split from apphub.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
)

// LocalToolRequest is one App-executed tool invocation.
type LocalToolRequest struct {
	InvocationID  string
	RunID         string
	ToolName      string
	ArgumentsJSON string
	TimeoutMS     int64
}

// LocalToolError is a structured App tool error.
type LocalToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

// LocalToolResult is the App's response to a local tool call.
type LocalToolResult struct {
	CorrelationID string
	InvocationID  string
	OK            bool
	ResultJSON    string
	Error         *LocalToolError
}

type pendingLocalCall struct {
	appID         string
	runID         string
	correlationID string
	result        chan LocalToolResult
}

// LocalToolResolution describes the outcome of one local tool result.
type LocalToolResolution int

type completedLocalCall struct {
	appID   string
	result  LocalToolResult
	expires time.Time
}

// CallLocalTool sends a local tool call and waits for the App result.
func (h *AppHub) CallLocalTool(ctx context.Context, appID string, request LocalToolRequest) (LocalToolResult, error) {
	if request.InvocationID == "" || request.ToolName == "" {
		return LocalToolResult{}, fmt.Errorf("invocation_id and tool_name are required")
	}
	msgID := id.New()
	pending := &pendingLocalCall{
		appID:         appID,
		runID:         request.RunID,
		correlationID: msgID,
		result:        make(chan LocalToolResult, 1),
	}
	h.mu.Lock()
	if _, exists := h.pending[request.InvocationID]; exists {
		h.mu.Unlock()
		return LocalToolResult{}, fmt.Errorf("local tool call %q is already pending", request.InvocationID)
	}
	h.pending[request.InvocationID] = pending
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, request.InvocationID)
		h.mu.Unlock()
	}()

	payload, err := json.Marshal(map[string]any{
		"type":           "local_tool_call",
		"msg_id":         msgID,
		"invocation_id":  request.InvocationID,
		"tool_name":      request.ToolName,
		"arguments_json": request.ArgumentsJSON,
		"timeout_ms":     request.TimeoutMS,
	})
	if err != nil {
		return LocalToolResult{}, fmt.Errorf("encode local tool call: %w", err)
	}
	if !h.Broadcast(appID, payload) {
		return LocalToolResult{}, ErrAppUnreachable
	}

	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-pending.result:
		if result.Error != nil && result.Error.Code == string(errs.CodeAppUnreachable) {
			return result, ErrAppUnreachable
		}
		return result, nil
	case <-ctx.Done():
		return LocalToolResult{}, ctx.Err()
	case <-timer.C:
		return LocalToolResult{}, ErrLocalToolTimeout
	}
}

// ResolveLocalTool delivers one App result to its waiting call.
// Duplicate results after the first are ignored.
func (h *AppHub) ResolveLocalTool(appID string, result LocalToolResult) bool {
	return h.ResolveLocalToolChecked(appID, result) == LocalToolResolved
}

// ResolveLocalToolChecked validates and delivers one local tool result.
func (h *AppHub) ResolveLocalToolChecked(appID string, result LocalToolResult) LocalToolResolution {
	if result.InvocationID == "" {
		return LocalToolUnknown
	}
	h.mu.Lock()
	pending := h.pending[result.InvocationID]
	if pending == nil {
		completed, ok := h.completed[result.InvocationID]
		h.mu.Unlock()
		if ok && completed.appID == appID {
			return LocalToolDuplicate
		}
		return LocalToolUnknown
	}
	if pending.appID != appID {
		h.mu.Unlock()
		return LocalToolForbidden
	}
	if result.CorrelationID == "" || result.CorrelationID != pending.correlationID {
		h.mu.Unlock()
		return LocalToolCorrelationMismatch
	}
	delete(h.pending, result.InvocationID)
	h.rememberCompletedLocked(result.InvocationID, appID, result)
	h.mu.Unlock()
	select {
	case pending.result <- result:
	default:
	}
	return LocalToolResolved
}

// CachedLocalToolResult returns a recently completed invocation result.
func (h *AppHub) CachedLocalToolResult(appID, invocationID string) (LocalToolResult, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	completed, ok := h.completed[invocationID]
	if !ok || completed.appID != appID || time.Now().After(completed.expires) {
		return LocalToolResult{}, false
	}
	return completed.result, true
}

func (h *AppHub) rememberCompletedLocked(invocationID, appID string, result LocalToolResult) {
	now := time.Now()
	for key, completed := range h.completed {
		if now.After(completed.expires) {
			delete(h.completed, key)
		}
	}
	if len(h.completed) >= 1024 {
		for key := range h.completed {
			delete(h.completed, key)
			break
		}
	}
	h.completed[invocationID] = completedLocalCall{
		appID:   appID,
		result:  result,
		expires: now.Add(10 * time.Minute),
	}
}

// CancelLocalTools fails pending local tool calls for one App/run.
func (h *AppHub) CancelLocalTools(appID, runID, code, message string) {
	h.failPendingWhere(func(call *pendingLocalCall) bool {
		return call.appID == appID && (runID == "" || call.runID == runID)
	}, code, message)
}

func (h *AppHub) PendingLocalTools(appID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for _, pending := range h.pending {
		if pending.appID == appID {
			count++
		}
	}
	return count
}

func (h *AppHub) failPending(appID, code, message string) {
	h.failPendingWhere(func(call *pendingLocalCall) bool {
		return call.appID == appID
	}, code, message)
}

func (h *AppHub) failPendingWhere(match func(*pendingLocalCall) bool, code, message string) {
	h.mu.Lock()
	pending := make([]*pendingLocalCall, 0)
	invocations := make([]string, 0)
	for invocationID, call := range h.pending {
		if match(call) {
			delete(h.pending, invocationID)
			pending = append(pending, call)
			invocations = append(invocations, invocationID)
		}
	}
	h.mu.Unlock()
	for index, call := range pending {
		select {
		case call.result <- LocalToolResult{
			InvocationID: invocations[index],
			OK:           false,
			Error:        &LocalToolError{Code: code, Message: message},
		}:
		default:
		}
	}
}
