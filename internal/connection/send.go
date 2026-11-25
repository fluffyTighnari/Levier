// Levier — connection
//
// Split from daemonhub.go (P8R.3).
// Copyright 2026 The Levier Authors.
package connection

import (
	"context"
	"fmt"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/protocol"
)

var (
	// ErrAckTimeout reports that a TaskAssign was not acknowledged in time.
	ErrAckTimeout = fmt.Errorf("task ack timed out")
)

// Dispatch sends one envelope and waits for its TaskAck on the same stream.
// The blocking result is what lets a split control replica avoid a
// process-local pending map.
func (h *Hub) Dispatch(
	ctx context.Context,
	daemonID string,
	env *runtimev1.Envelope,
	timeout time.Duration,
) (*runtimev1.TaskAck, error) {
	if env == nil {
		return nil, fmt.Errorf("envelope is nil")
	}
	if env.GetMsgId() == "" {
		return nil, fmt.Errorf("dispatch msg_id is required")
	}
	if timeout <= 0 {
		timeout = h.cfg.SendTimeout + 5*time.Second
	}
	ackCh := make(chan *runtimev1.TaskAck, 1)
	h.ackMu.Lock()
	if _, exists := h.acks[env.GetMsgId()]; exists {
		h.ackMu.Unlock()
		return nil, fmt.Errorf("dispatch %q is already pending", env.GetMsgId())
	}
	h.acks[env.GetMsgId()] = ackCh
	h.ackMu.Unlock()
	defer func() {
		h.ackMu.Lock()
		delete(h.acks, env.GetMsgId())
		h.ackMu.Unlock()
	}()

	if err := h.Send(daemonID, env); err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ack := <-ackCh:
		return ack, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrAckTimeout
	}
}

func (h *Hub) resolveAck(correlationID string, ack *runtimev1.TaskAck) {
	if correlationID == "" {
		return
	}
	h.ackMu.Lock()
	ch := h.acks[correlationID]
	h.ackMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- ack:
	default:
	}
}

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
