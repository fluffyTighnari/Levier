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

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/middleware/internal/connection"
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
	return websocket.Handler(handler)
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
			appConn.Subscribe(typed.RunID)
			attempt, seq, err := s.replayWS(appConn, typed)
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
				InvocationID: typed.InvocationID,
				OK:           typed.OK,
				ResultJSON:   typed.ResultJSON,
			}
			if typed.Error != nil {
				result.Error = &connection.LocalToolError{
					Code:      typed.Error.Code,
					Message:   typed.Error.Message,
					Retryable: typed.Error.Retryable,
				}
			}
			s.appHub.ResolveLocalTool(appID, result)
		}
	}
}

func (s *Server) replayWS(appConn *connection.AppConn, msg *SubscribeMessage) (int32, int64, error) {
	events, err := s.runs.ListEvents(context.Background(), msg.RunID, msg.LastAttempt, msg.LastSeq, 1000)
	if err != nil {
		return 0, 0, err
	}
	attempt, seq := msg.LastAttempt, msg.LastSeq
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
		if err := s.sendWS(appConn, view); err != nil {
			return attempt, seq, err
		}
		attempt, seq = int32(event.Attempt), event.Seq
	}
	return attempt, seq, nil
}

func (s *Server) sendWS(appConn *connection.AppConn, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return appConn.Send(payload)
}
