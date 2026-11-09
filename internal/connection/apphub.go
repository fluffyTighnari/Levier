// Middleware — internal/connection
//
// AppHub manages App WebSocket connections, subscriptions, and event fanout.
//
// Copyright 2026 The Middleware Authors.
package connection

import (
	"sync"
)

// EventSink is the transport-neutral App connection writer.
type EventSink interface {
	Send(payload []byte) error
	Close() error
}

// AppConn is one registered App WebSocket connection.
type AppConn struct {
	appID string
	sink  EventSink

	mu            sync.RWMutex
	subscriptions map[string]struct{}
	closed        bool
}

// AppHub tracks App connections and run subscriptions.
type AppHub struct {
	mu    sync.RWMutex
	apps  map[string]map[*AppConn]struct{}
	conns map[*AppConn]struct{}
}

// NewAppHub returns an empty AppHub.
func NewAppHub() *AppHub {
	return &AppHub{
		apps:  make(map[string]map[*AppConn]struct{}),
		conns: make(map[*AppConn]struct{}),
	}
}

// Register adds one App connection.
func (h *AppHub) Register(appID string, sink EventSink) *AppConn {
	conn := &AppConn{
		appID:         appID,
		sink:          sink,
		subscriptions: make(map[string]struct{}),
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

// Send writes one payload to the connection sink.
func (c *AppConn) Send(payload []byte) error {
	return c.sink.Send(payload)
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
	h.mu.Unlock()
	conn.close()
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
		if err := conn.sink.Send(payload); err != nil {
			h.CloseConn(conn)
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
	h.mu.Unlock()
	for _, conn := range conns {
		conn.close()
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
	_ = c.sink.Close()
}
