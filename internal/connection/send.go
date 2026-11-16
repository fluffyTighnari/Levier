// Middleware — connection
//
// Split from daemonhub.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package connection

import (
	"fmt"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/protocol"
)

// Send queues one envelope to a daemon.
func (h *Hub) Send(daemonID string, env *runtimev1.Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	if err := protocol.Validate(env); err != nil {
		return err
	}

	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return ErrHubClosed
	}
	conn := h.daemons[daemonID]
	h.mu.RUnlock()
	if conn == nil {
		return ErrDaemonOffline
	}

	select {
	case <-conn.done:
		return ErrDaemonOffline
	default:
	}

	if ClassifyEnvelope(env) == SendEphemeral {
		select {
		case conn.sendCh <- env:
			return nil
		case <-conn.done:
			return ErrDaemonOffline
		default:
			conn.droppedEphemeral.Add(1)
			return nil
		}
	}

	timer := time.NewTimer(h.cfg.SendTimeout)
	defer timer.Stop()
	select {
	case conn.sendCh <- env:
		conn.clearSlow()
		return nil
	case <-conn.done:
		return ErrDaemonOffline
	case <-timer.C:
		conn.markSlow(h.cfg.SlowConsumerTimeout)
		return ErrBackpressure
	}
}

// Disconnect closes one daemon stream. The channel handler then removes the
// daemon and fires OnOffline, which requeues its active runs.
func (h *Hub) Disconnect(daemonID string) {
	h.mu.RLock()
	conn := h.daemons[daemonID]
	h.mu.RUnlock()
	if conn != nil {
		conn.close()
	}
}

// ClassifyEnvelope classifies an outbound envelope for backpressure.
func ClassifyEnvelope(env *runtimev1.Envelope) SendKind {
	if env == nil {
		return SendControl
	}
	if event := env.GetEvent(); event != nil {
		if event.GetEphemeral() {
			return SendEphemeral
		}
		return SendPersistent
	}
	return SendControl
}

// DroppedEphemeral returns the number of dropped ephemeral events for a daemon.
func (h *Hub) DroppedEphemeral(daemonID string) int64 {
	h.mu.RLock()
	conn := h.daemons[daemonID]
	h.mu.RUnlock()
	if conn == nil {
		return 0
	}
	return conn.droppedEphemeral.Load()
}

func (c *daemonConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case env := <-c.sendCh:
			if err := c.stream.Send(env); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *daemonConn) markSlow(timeout time.Duration) {
	c.slowMu.Lock()
	c.slow = true
	if c.slowTimer == nil {
		c.slowTimer = time.AfterFunc(timeout, func() {
			c.slowMu.Lock()
			slow := c.slow
			c.slowMu.Unlock()
			if slow {
				c.close()
			}
		})
	}
	c.slowMu.Unlock()
}

func (c *daemonConn) clearSlow() {
	c.slowMu.Lock()
	c.slow = false
	if c.slowTimer != nil {
		c.slowTimer.Stop()
		c.slowTimer = nil
	}
	c.slowMu.Unlock()
}

func (c *daemonConn) close() {
	c.once.Do(func() {
		close(c.done)
		c.slowMu.Lock()
		c.slow = false
		if c.slowTimer != nil {
			c.slowTimer.Stop()
			c.slowTimer = nil
		}
		c.slowMu.Unlock()
	})
}
