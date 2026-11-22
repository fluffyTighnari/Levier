// Levier — internal/httpapi
//
// App WebSocket connection handling.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/fluffyTighnari/levier/internal/contract"
	"github.com/fluffyTighnari/levier/internal/errs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/observe"
)

const (
	wsSinkQueueMessages = 4096
	wsSinkQueueBytes    = 4 << 20
	wsSinkWriteTimeout  = 5 * time.Second
)

var errWSSinkClosed = errors.New("websocket sink is closed")

// wsSink decouples Control from socket writes. A subscriber that stops reading
// fills the bounded queue and is disconnected instead of stalling the event loop.
type wsSink struct {
	conn  *websocket.Conn
	appID string
	log   *slog.Logger

	queue   chan []byte
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	pending int
	closed  bool
}

func newWSSink(conn *websocket.Conn, appID string, log *slog.Logger) *wsSink {
	sink := &wsSink{
		conn:  conn,
		appID: appID,
		log:   log,
		queue: make(chan []byte, wsSinkQueueMessages),
		done:  make(chan struct{}),
	}
	go sink.writeLoop()
	return sink
}

func (s *wsSink) Send(payload []byte) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errWSSinkClosed
	}
	if s.pending+len(payload) > wsSinkQueueBytes {
		pending := s.pending
		s.mu.Unlock()
		s.warnSlowConsumer(pending, len(payload))
		_ = s.Close()
		return errWSSinkClosed
	}
	s.pending += len(payload)
	select {
	case s.queue <- payload:
		s.mu.Unlock()
		return nil
	default:
		s.pending -= len(payload)
		s.mu.Unlock()
		s.warnSlowConsumer(s.pendingOf(), len(payload))
		_ = s.Close()
		return errWSSinkClosed
	}
}

func (s *wsSink) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case payload := <-s.queue:
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(wsSinkWriteTimeout))
			if err := websocket.Message.Send(s.conn, string(payload)); err != nil {
				_ = s.Close()
				return
			}
			s.release(len(payload))
		}
	}
}

func (s *wsSink) release(size int) {
	s.mu.Lock()
	s.pending -= size
	if s.pending < 0 {
		s.pending = 0
	}
	s.mu.Unlock()
}

func (s *wsSink) pendingOf() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

func (s *wsSink) warnSlowConsumer(pending, payload int) {
	if s.log != nil {
		s.log.Warn("app websocket slow consumer",
			observe.FieldAppID, s.appID,
			"pending_bytes", pending,
			"payload_bytes", payload,
		)
	}
}

func (s *wsSink) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
		_ = s.conn.Close()
	})
	return nil
}

func (s *Server) websocketHandler(handler func(*websocket.Conn)) http.Handler {
	// Validate app_id/token before the Upgrade so expired or cross-App tokens
	// receive an HTTP rejection instead of a socket that closes immediately.
	// The x/net Server still gets a permissive handshake so non-browser
	// clients that omit Origin are accepted after token validation.
	websocketServer := websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler:   handler,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appID := r.URL.Query().Get("app_id")
		token := r.URL.Query().Get("token")
		tokenAppID, ok := s.wsTokens.Validate(token)
		if !ok || appID == "" || tokenAppID != appID {
			http.Error(w, "invalid ws token", http.StatusForbidden)
			return
		}
		websocketServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleWS(conn *websocket.Conn) {
	query := conn.Request().URL.Query()
	appID := query.Get("app_id")
	token := query.Get("token")
	tokenAppID, ok := s.wsTokens.Validate(token)
	if !ok || appID == "" || tokenAppID != appID {
		_ = conn.Close()
		return
	}

	tenantID := s.tenantForApp(conn.Request().Context(), appID)
	started := time.Now()
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.AddGauge(observe.MetricWSConnections, "Current App WebSocket connections.", map[string]string{
			"app": appID, "tenant": tenantID,
		}, 1)
		defer s.cfg.Metrics.AddGauge(observe.MetricWSConnections, "Current App WebSocket connections.", map[string]string{
			"app": appID, "tenant": tenantID,
		}, -1)
	}
	s.log.Info("ws connected",
		observe.FieldAppID, appID,
		observe.FieldTenantID, tenantID,
		observe.FieldRemoteAddr, conn.Request().RemoteAddr,
	)
	defer func() {
		s.log.Info("ws disconnected",
			observe.FieldAppID, appID,
			observe.FieldDurationMS, time.Since(started).Milliseconds(),
		)
	}()

	appConn := s.appHub.Register(appID, newWSSink(conn, appID, s.log))
	defer s.appHub.CloseConn(appConn)

	for {
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return
		}
		message, err := ParseClientMessage([]byte(raw))
		if err != nil {
			_ = s.sendWS(appConn, ErrorMessage{
				Type:  contract.WSMessageError,
				Error: ErrorBody{Code: string(errs.CodeBadMessage), Message: err.Error()},
			})
			continue
		}
		switch typed := message.(type) {
		case *HelloMessage:
			continue
		case *PingMessage:
			_ = s.sendWS(appConn, PongMessage{Type: contract.WSMessagePong})
		case *SubscribeMessage:
			run, err := s.runs.GetRun(context.Background(), typed.RunID)
			if err != nil || run.AppID != appID {
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeRunNotFound), Message: "run not found"},
				})
				continue
			}
			appConn.BeginReplay(typed.RunID, typed.LastAttempt, typed.LastSeq)
			appConn.Subscribe(typed.RunID)
			attempt, seq, err := s.replayWS(appConn, typed)
			if endErr := appConn.EndReplay(typed.RunID, attempt, seq); err == nil {
				err = endErr
			}
			if err != nil {
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeReplayFailed), Message: err.Error()},
				})
				continue
			}
			_ = s.sendWS(appConn, CaughtUpMessage{
				Type: contract.WSMessageCaughtUp, RunID: typed.RunID,
				Attempt: attempt, Seq: seq,
			})
		case *LocalToolResultMessage:
			result := connection.LocalToolResult{
				CorrelationID: typed.CorrelationID,
				InvocationID:  typed.InvocationID,
				OK:            typed.OK,
				ResultJSON:    typed.ResultJSON,
			}
			if typed.Error != nil {
				result.Error = &connection.LocalToolError{
					Code:      typed.Error.Code,
					Message:   typed.Error.Message,
					Retryable: typed.Error.Retryable,
				}
			}
			switch s.appHub.ResolveLocalToolChecked(appID, result) {
			case connection.LocalToolUnknown:
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeLocalToolUnknown), Message: "local tool invocation is not pending"},
				})
			case connection.LocalToolForbidden:
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeLocalToolForbidden), Message: "local tool invocation belongs to another app"},
				})
			case connection.LocalToolCorrelationMismatch:
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeLocalToolCorrelationMismatch), Message: "correlation_id does not match the local tool call"},
				})
			}
		}
	}
}

const wsReplayPageSize = 1000

func (s *Server) replayWS(appConn *connection.AppConn, msg *SubscribeMessage) (int32, int64, error) {
	attempt, seq := msg.LastAttempt, msg.LastSeq
	for {
		events, err := s.runs.ListEvents(context.Background(), msg.RunID, attempt, seq, wsReplayPageSize)
		if err != nil {
			return attempt, seq, err
		}
		for _, event := range events {
			view := EventMessage{
				Type:      contract.WSMessageEvent,
				RunID:     event.RunID,
				Attempt:   int32(event.Attempt),
				Seq:       event.Seq,
				Source:    event.Source,
				EventType: event.Type,
				Data:      event.Payload,
				TSMillis:  event.TSMs,
			}
			payload, err := json.Marshal(view)
			if err != nil {
				return attempt, seq, err
			}
			if err := appConn.SendReplay(msg.RunID, int32(event.Attempt), event.Seq, payload); err != nil {
				return attempt, seq, err
			}
			attempt, seq = int32(event.Attempt), event.Seq
		}
		if len(events) < wsReplayPageSize {
			return attempt, seq, nil
		}
	}
}

func (s *Server) sendWS(appConn *connection.AppConn, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return appConn.Send(payload)
}
