// Middleware — internal/connection
//
// AppHub manages App WebSocket connections, subscriptions, and event fanout.
//
// Copyright 2026 The Middleware Authors.
package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"sync"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
)

// EventSink is the transport-neutral App connection writer.
type EventSink interface {
	Send(payload []byte) error
	Close() error
}

var (
	// ErrAppUnreachable reports that no App WebSocket connection is available.
	ErrAppUnreachable = errors.New("app is unreachable")
	// ErrLocalToolTimeout reports a local tool call deadline.
	ErrLocalToolTimeout = errors.New("local tool call timed out")
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

const (
	// LocalToolResolved means the pending call received the result.
	LocalToolResolved LocalToolResolution = iota
	// LocalToolDuplicate means the invocation already completed.
	LocalToolDuplicate
	// LocalToolUnknown means no pending or completed invocation matched.
	LocalToolUnknown
	// LocalToolForbidden means the result came from another App.
	LocalToolForbidden
	// LocalToolCorrelationMismatch means correlation_id does not match.
	LocalToolCorrelationMismatch
)

type completedLocalCall struct {
	appID   string
	result  LocalToolResult
	expires time.Time
}

// runCursor is the last (attempt, seq) delivered to one run stream.
type runCursor struct {
	attempt int32
	seq     int64
}

// replayBuffer holds live payloads that arrive while replay is running.
type replayBuffer struct {
	payloads [][]byte
}

// AppConn is one registered App WebSocket connection.
type AppConn struct {
	appID string
	sink  EventSink

	mu            sync.RWMutex
	subscriptions map[string]struct{}
	cursors       map[string]runCursor
	replays       map[string]*replayBuffer
	closed        bool

	sendMu sync.Mutex
}

// AppHub tracks App connections and run subscriptions.
type AppHub struct {
	mu        sync.RWMutex
	apps      map[string]map[*AppConn]struct{}
	conns     map[*AppConn]struct{}
	pending   map[string]*pendingLocalCall
	completed map[string]completedLocalCall
}

// NewAppHub returns an empty AppHub.
func NewAppHub() *AppHub {
	return &AppHub{
		apps:      make(map[string]map[*AppConn]struct{}),
		conns:     make(map[*AppConn]struct{}),
		pending:   make(map[string]*pendingLocalCall),
		completed: make(map[string]completedLocalCall),
	}
}

// Register adds one App connection.
func (h *AppHub) Register(appID string, sink EventSink) *AppConn {
	conn := &AppConn{
		appID:         appID,
		sink:          sink,
		subscriptions: make(map[string]struct{}),
		cursors:       make(map[string]runCursor),
		replays:       make(map[string]*replayBuffer),
	}
	h.mu.Lock()
	if h.apps[appID] == nil {
		h.apps[appID] = make(map[*AppConn]struct{})
	}
	h.apps[appID][conn] = struct{}{}
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
	return conn
}

// Subscribe subscribes one connection to a run.
func (c *AppConn) Subscribe(runID string) {
	c.mu.Lock()
	c.subscriptions[runID] = struct{}{}
	c.mu.Unlock()
}

// Unsubscribe removes a run subscription.
func (c *AppConn) Unsubscribe(runID string) {
	c.mu.Lock()
	delete(c.subscriptions, runID)
	c.mu.Unlock()
}

// Send writes one non-stream payload to the connection sink.
func (c *AppConn) Send(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.sendLocked(payload)
}

// BeginReplay starts buffering live events for one run.
func (c *AppConn) BeginReplay(runID string, attempt int32, seq int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replays[runID] = &replayBuffer{}
	if cursorNewer(attempt, seq, c.cursors[runID]) {
		c.cursors[runID] = runCursor{attempt: attempt, seq: seq}
	}
}

// SendReplay writes one replayed event and advances the run cursor.
func (c *AppConn) SendReplay(runID string, attempt int32, seq int64, payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.RLock()
	cursor := c.cursors[runID]
	c.mu.RUnlock()
	if !cursorNewer(attempt, seq, cursor) {
		return nil
	}
	if err := c.sendLocked(payload); err != nil {
		return err
	}
	c.advance(runID, attempt, seq)
	return nil
}

// SendRun writes one live run payload, buffering while a replay is active.
func (c *AppConn) SendRun(runID string, payload []byte) error {
	attempt, seq, hasCursor := payloadCursor(payload)
	c.mu.Lock()
	if state := c.replays[runID]; state != nil {
		if !hasCursor || cursorNewer(attempt, seq, c.cursors[runID]) {
			state.payloads = append(state.payloads, payload)
		}
		c.mu.Unlock()
		return nil
	}
	if hasCursor && !cursorNewer(attempt, seq, c.cursors[runID]) {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if hasCursor {
		c.mu.RLock()
		cursor := c.cursors[runID]
		c.mu.RUnlock()
		if !cursorNewer(attempt, seq, cursor) {
			return nil
		}
	}
	if err := c.sendLocked(payload); err != nil {
		return err
	}
	if hasCursor {
		c.advance(runID, attempt, seq)
	}
	return nil
}

// EndReplay flushes buffered live events newer than the replay watermark.
func (c *AppConn) EndReplay(runID string, attempt int32, seq int64) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.Lock()
	state := c.replays[runID]
	delete(c.replays, runID)
	if cursorNewer(attempt, seq, c.cursors[runID]) {
		c.cursors[runID] = runCursor{attempt: attempt, seq: seq}
	}
	c.mu.Unlock()
	if state == nil {
		return nil
	}

	for _, payload := range state.payloads {
		eventAttempt, eventSeq, hasCursor := payloadCursor(payload)
		if hasCursor {
			c.mu.RLock()
			cursor := c.cursors[runID]
			c.mu.RUnlock()
			if !cursorNewer(eventAttempt, eventSeq, cursor) {
				continue
			}
		}
		if err := c.sendLocked(payload); err != nil {
			return err
		}
		if hasCursor {
			c.advance(runID, eventAttempt, eventSeq)
		}
	}
	return nil
}

func (c *AppConn) sendLocked(payload []byte) error {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return fmt.Errorf("app connection is closed")
	}
	return c.sink.Send(payload)
}

func (c *AppConn) advance(runID string, attempt int32, seq int64) {
	c.mu.Lock()
	if cursorNewer(attempt, seq, c.cursors[runID]) {
		c.cursors[runID] = runCursor{attempt: attempt, seq: seq}
	}
	c.mu.Unlock()
}

// AppID returns the connection's App identity.
func (c *AppConn) AppID() string {
	return c.appID
}

// Close removes one connection from the hub and closes its sink.
func (h *AppHub) CloseConn(conn *AppConn) {
	if conn == nil {
		return
	}
	h.mu.Lock()
	if set := h.apps[conn.appID]; set != nil {
		delete(set, conn)
		if len(set) == 0 {
			delete(h.apps, conn.appID)
		}
	}
	delete(h.conns, conn)
	remaining := len(h.apps[conn.appID])
	h.mu.Unlock()
	conn.close()
	if remaining == 0 {
		h.failPending(conn.appID, string(errs.CodeAppUnreachable), "app connection closed")
	}
}

// Publish sends a JSON payload to App connections subscribed to runID.
func (h *AppHub) Publish(appID, runID string, payload []byte) {
	h.mu.RLock()
	conns := make([]*AppConn, 0)
	for conn := range h.apps[appID] {
		conn.mu.RLock()
		_, subscribed := conn.subscriptions[runID]
		conn.mu.RUnlock()
		if subscribed {
			conns = append(conns, conn)
		}
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		if err := conn.SendRun(runID, payload); err != nil {
			h.CloseConn(conn)
		}
	}
}

// Broadcast sends a payload to every connection of one App.
func (h *AppHub) Broadcast(appID string, payload []byte) bool {
	h.mu.RLock()
	conns := make([]*AppConn, 0, len(h.apps[appID]))
	for conn := range h.apps[appID] {
		conns = append(conns, conn)
	}
	h.mu.RUnlock()
	delivered := false
	for _, conn := range conns {
		if err := conn.sink.Send(payload); err != nil {
			h.CloseConn(conn)
			continue
		}
		delivered = true
	}
	return delivered
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

// Close closes every App connection.
func (h *AppHub) Close() {
	h.mu.Lock()
	conns := make([]*AppConn, 0, len(h.conns))
	for conn := range h.conns {
		conns = append(conns, conn)
	}
	h.apps = make(map[string]map[*AppConn]struct{})
	h.conns = make(map[*AppConn]struct{})
	pendingApps := make(map[string]struct{})
	for _, call := range h.pending {
		pendingApps[call.appID] = struct{}{}
	}
	h.mu.Unlock()
	for _, conn := range conns {
		conn.close()
	}
	for appID := range pendingApps {
		h.failPending(appID, string(errs.CodeAppUnreachable), "app hub closed")
	}
}

// Count returns the number of App connections for one app.
func (h *AppHub) Count(appID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.apps[appID])
}

func (c *AppConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	_ = c.sink.Close()
}

func payloadCursor(payload []byte) (int32, int64, bool) {
	var probe struct {
		Attempt int32 `json:"attempt"`
		Seq     int64 `json:"seq"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil || probe.Seq <= 0 {
		return 0, 0, false
	}
	return probe.Attempt, probe.Seq, true
}

func cursorNewer(attempt int32, seq int64, cursor runCursor) bool {
	return attempt > cursor.attempt || (attempt == cursor.attempt && seq > cursor.seq)
}
