// Levier — connection
//
// Split from daemonhub.go (P8R.3).
// Copyright 2026 The Levier Authors.
package connection

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
)

func (h *Hub) receiveHello(stream runtimev1.RuntimeService_ChannelServer) (*runtimev1.Envelope, error) {
	type result struct {
		env *runtimev1.Envelope
		err error
	}
	ch := make(chan result, 1)
	go func() {
		env, err := stream.Recv()
		ch <- result{env: env, err: err}
	}()

	timer := time.NewTimer(h.cfg.HelloTimeout)
	defer timer.Stop()
	select {
	case <-stream.Context().Done():
		return nil, stream.Context().Err()
	case <-timer.C:
		return nil, status.Errorf(codes.DeadlineExceeded, "hello not received within %s", h.cfg.HelloTimeout)
	case got := <-ch:
		if got.err != nil {
			if errors.Is(got.err, io.EOF) {
				return nil, status.Error(codes.InvalidArgument, "stream closed before hello")
			}
			return nil, got.err
		}
		return got.env, nil
	}
}

func (h *Hub) readLoop(conn *daemonConn) error {
	for {
		type received struct {
			env *runtimev1.Envelope
			err error
		}
		ch := make(chan received, 1)
		go func() {
			env, err := conn.stream.Recv()
			ch <- received{env: env, err: err}
		}()

		var got received
		select {
		case <-conn.done:
			return nil
		case <-conn.stream.Context().Done():
			return nil
		case got = <-ch:
		}
		if got.err != nil {
			if errors.Is(got.err, io.EOF) {
				return nil
			}
			return got.err
		}
		env := got.env
		if err := protocol.Validate(env); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		switch payload := env.GetPayload().(type) {
		case *runtimev1.Envelope_Hello:
			return status.Error(codes.InvalidArgument, "duplicate hello")
		case *runtimev1.Envelope_Heartbeat:
			count := payload.Heartbeat.GetRunningCount()
			h.updateHeartbeat(conn, count)
			h.mu.RLock()
			heartbeat := h.onHeartbeat
			h.mu.RUnlock()
			h.safeCall(func() {
				if heartbeat != nil {
					heartbeat(conn.info.DaemonID, count)
				}
			})
		default:
			if ack := env.GetTaskAck(); ack != nil {
				h.resolveAck(env.GetCorrelationId(), ack)
			}
			h.mu.RLock()
			handler := h.onEnvelope
			h.mu.RUnlock()
			h.safeCall(func() {
				if handler != nil {
					handler(conn.info.DaemonID, env)
				}
			})
		}
	}
}

func (h *Hub) updateHeartbeat(conn *daemonConn, runningCount int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.daemons[conn.info.DaemonID]
	if current != conn {
		return
	}
	conn.info.LastHeartbeat = h.cfg.Now()
	if runningCount >= 0 {
		conn.info.RunningCount = runningCount
	}
}

func (h *Hub) notifyHello(info DaemonInfo) {
	h.mu.RLock()
	handler := h.onHello
	h.mu.RUnlock()
	h.safeCall(func() {
		if handler != nil {
			handler(info)
		}
	})
}

func (h *Hub) notifyOffline(daemonID string) {
	h.notifyOfflineInfo(DaemonInfo{DaemonID: daemonID})
}

func (h *Hub) notifyOfflineInfo(info DaemonInfo) {
	h.mu.RLock()
	infoHandler := h.onOfflineInfo
	handler := h.onOffline
	h.mu.RUnlock()
	h.safeCall(func() {
		if infoHandler != nil {
			infoHandler(info)
			return
		}
		if handler != nil {
			handler(info.DaemonID)
		}
	})
}

func (h *Hub) safeCall(fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("daemon hub callback panic: %v", recovered)
			slog.Error("daemon hub callback panic", "err", err)
			if h.cfg.OnError != nil {
				h.cfg.OnError(err)
			}
		}
	}()
	fn()
}

func (h *Hub) healthy(info DaemonInfo, now time.Time) bool {
	last := info.LastHeartbeat
	if last.IsZero() {
		last = info.ConnectedAt
	}
	return now.Sub(last) <= h.cfg.HeartbeatTimeout
}

func effectiveRunning(info DaemonInfo) int32 {
	if info.RunningCount > info.Reserved {
		return info.RunningCount
	}
	return info.Reserved
}
