// Middleware — connection
//
// Split from apphub.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package connection

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

const (
	maxReplayPayloads = 1024
	maxReplayBytes    = 4 << 20
)

// ErrReplayOverflow reports that live events exceeded the replay buffer.
var ErrReplayOverflow = errors.New("app replay buffer overflow")

// runCursor is the last (attempt, seq) delivered to one run stream.
type runCursor struct {
	attempt int32
	seq     int64
}

// replayBuffer holds live payloads that arrive while replay is running.
type replayBuffer struct {
	payloads [][]byte
	bytes    int
	overflow bool
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
		if hasCursor && !cursorNewer(attempt, seq, c.cursors[runID]) {
			c.mu.Unlock()
			return nil
		}
		if state.overflow ||
			len(state.payloads) >= maxReplayPayloads ||
			state.bytes+len(payload) > maxReplayBytes {
			state.overflow = true
			c.mu.Unlock()
			return ErrReplayOverflow
		}
		state.payloads = append(state.payloads, payload)
		state.bytes += len(payload)
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
	if state.overflow {
		return ErrReplayOverflow
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
