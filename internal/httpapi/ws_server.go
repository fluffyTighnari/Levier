// Middleware — internal/httpapi
//
// App WebSocket connection handling.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/observe"
)

type wsSink struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (s *wsSink) Send(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return websocket.Message.Send(s.conn, string(payload))
}

func (s *wsSink) Close() error {
	return s.conn.Close()
}

func websocketHandler(handler func(*websocket.Conn)) http.Handler {
	// App auth is the ws_token query parameter. The default x/net Handler
	// rejects non-browser clients that omit Origin, so accept the handshake
	// here and let handleWS validate app_id/token.
	return websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler:   handler,
	}
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

	started := time.Now()
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.AddGauge(observe.MetricWSConnections, "Current App WebSocket connections.", map[string]string{
			"app": appID,
		}, 1)
		defer s.cfg.Metrics.AddGauge(observe.MetricWSConnections, "Current App WebSocket connections.", map[string]string{
			"app": appID,
		}, -1)
	}
	s.log.Info("ws connected",
		observe.FieldAppID, appID,
		observe.FieldRemoteAddr, conn.Request().RemoteAddr,
	)
	defer func() {
		s.log.Info("ws disconnected",
			observe.FieldAppID, appID,
			observe.FieldDurationMS, time.Since(started).Milliseconds(),
		)
	}()

	appConn := s.appHub.Register(appID, &wsSink{conn: conn})
	defer s.appHub.CloseConn(appConn)

	for {
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return
		}
		message, err := ParseClientMessage([]byte(raw))
		if err != nil {
			_ = s.sendWS(appConn, ErrorMessage{
				Type:  WSMessageError,
				Error: ErrorBody{Code: "BAD_MESSAGE", Message: err.Error()},
			})
			continue
		}
		switch typed := message.(type) {
		case *HelloMessage:
			continue
		case *PingMessage:
			_ = s.sendWS(appConn, PongMessage{Type: WSMessagePong})
		case *SubscribeMessage:
			run, err := s.runs.GetRun(context.Background(), typed.RunID)
			if err != nil || run.AppID != appID {
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  WSMessageError,
					Error: ErrorBody{Code: "RUN_NOT_FOUND", Message: "run not found"},
				})
				continue
			}
			appConn.BeginReplay(typed.RunID, typed.LastAttempt, typed.LastSeq)
			appConn.Subscribe(typed.RunID)
			attempt, seq, err := s.replayWS(appConn, typed)
			_ = appConn.EndReplay(typed.RunID, attempt, seq)
			if err != nil {
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  WSMessageError,
					Error: ErrorBody{Code: "REPLAY_FAILED", Message: err.Error()},
				})
				continue
			}
			_ = s.sendWS(appConn, CaughtUpMessage{
				Type: WSMessageCaughtUp, RunID: typed.RunID,
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
					Type:  WSMessageError,
					Error: ErrorBody{Code: "LOCAL_TOOL_UNKNOWN", Message: "local tool invocation is not pending"},
				})
			case connection.LocalToolForbidden:
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  WSMessageError,
					Error: ErrorBody{Code: "LOCAL_TOOL_FORBIDDEN", Message: "local tool invocation belongs to another app"},
				})
			case connection.LocalToolCorrelationMismatch:
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  WSMessageError,
					Error: ErrorBody{Code: "LOCAL_TOOL_CORRELATION_MISMATCH", Message: "correlation_id does not match the local tool call"},
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
				Type:      WSMessageEvent,
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
