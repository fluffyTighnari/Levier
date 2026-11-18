// Levier — connection
//
// Split from apphub.go (P8R.3).
// Copyright 2026 The Levier Authors.
package connection

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/fluffyTighnari/levier/internal/errs"
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
			if errors.Is(err, ErrReplayOverflow) {
				slog.Warn("app replay buffer overflow",
					"app_id", appID, "run_id", runID,
				)
			}
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
		if err := conn.Send(payload); err != nil {
			h.CloseConn(conn)
			continue
		}
		delivered = true
	}
	return delivered
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

// Count returns the number of App WebSocket connections for one app.
func (h *AppHub) Count(appID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.apps[appID])
}

// AppStats is an aggregate connection snapshot.
type AppStats struct {
	Apps        int
	Connections int
}

// Stats returns the number of distinct Apps and active connections.
func (h *AppHub) Stats() AppStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return AppStats{Apps: len(h.apps), Connections: len(h.conns)}
}
