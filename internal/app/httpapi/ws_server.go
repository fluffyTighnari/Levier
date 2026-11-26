// Levier — internal/app/httpapi
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
	"github.com/fluffyTighnari/levier/internal/platform/errs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
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
	connectionID := id.New()
	s.log.Info("ws connected",
		observe.FieldAppID, appID,
		"connection_id", connectionID,
		observe.FieldTenantID, tenantID,
		observe.FieldRemoteAddr, conn.Request().RemoteAddr,
	)
	defer func() {
		s.log.Info("ws disconnected",
			observe.FieldAppID, appID,
			"connection_id", connectionID,
			observe.FieldDurationMS, time.Since(started).Milliseconds(),
		)
	}()

	appConn := s.appHub.RegisterWithID(appID, newWSSink(conn, appID, s.log), connectionID)
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
			if err := s.handleHello(appConn, appID, typed); err != nil {
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: string(errs.CodeBadMessage), Message: err.Error()},
				})
			}
		case *PingMessage:
			_ = s.sendWS(appConn, PongMessage{Type: contract.WSMessagePong})
		case *SubscribeMessage:
			if err := s.subscribeAndReplay(appConn, appID, typed.RunID, typed.LastAttempt, typed.LastSeq); err != nil {
				code := string(errs.CodeReplayFailed)
				message := err.Error()
				if errors.Is(err, errRunNotFound) {
					code = string(errs.CodeRunNotFound)
					message = "run not found"
				}
				_ = s.sendWS(appConn, ErrorMessage{
					Type:  contract.WSMessageError,
					Error: ErrorBody{Code: code, Message: message},
				})
			}
		case *LocalToolProgressMessage:
			s.log.Info("app local tool progress",
				observe.FieldAppID, appID,
				"connection_id", appConn.ConnectionID(),
				observe.FieldInvocationID, typed.InvocationID,
				"progress", string(typed.Progress),
			)
		case *LocalToolResultMessage:
			result := connection.LocalToolResult{
				CorrelationID: typed.CorrelationID,
				InvocationID:  typed.InvocationID,
				ConnectionID:  appConn.ConnectionID(),
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

const (
	wsReplayPageSize      = 1000
	wsHeartbeatIntervalMS = 20000
)

var errRunNotFound = errors.New("run not found")

type appConnectionMetadataUpdater interface {
	UpdateConnectionMetadata(appID, connectionID string, protocolVersion int, capabilities []string)
}

func (s *Server) handleHello(appConn *connection.AppConn, appID string, msg *HelloMessage) error {
	if msg.AppID != "" && msg.AppID != appID {
		return errors.New("hello.app_id does not match the authenticated app")
	}
	version := msg.NegotiatedVersion()
	appConn.SetProtocol(version)
	appConn.SetCapabilities(msg.Capabilities)
	if updater, ok := s.appHub.(appConnectionMetadataUpdater); ok {
		updater.UpdateConnectionMetadata(appID, appConn.ConnectionID(), version, msg.Capabilities)
	}
	if version != contract.WSProtocolVersionV2 {
		return nil
	}

	if err := s.sendWS(appConn, HelloAckMessage{
		Type:                contract.WSMessageHelloAck,
		Protocol:            contract.WSProtocolVersionV2,
		ConnectionID:        appConn.ConnectionID(),
		AppID:               appID,
		Capabilities:        serverWSCapabilities(),
		HeartbeatIntervalMS: wsHeartbeatIntervalMS,
	}); err != nil {
		return err
	}
	if msg.ResumeCursor == nil {
		return nil
	}
	return s.subscribeAndReplay(
		appConn,
		appID,
		msg.ResumeCursor.RunID,
		msg.ResumeCursor.Attempt,
		msg.ResumeCursor.Seq,
	)
}

func serverWSCapabilities() []string {
	return []string{
		contract.WSCapabilityToolCallV2,
		contract.WSCapabilityProgress,
		contract.WSCapabilityCancel,
		contract.WSCapabilityIdempotentResult,
		contract.WSCapabilityResumeCursor,
	}
}

func (s *Server) subscribeAndReplay(
	appConn *connection.AppConn,
	appID, runID string,
	lastAttempt int32,
	lastSeq int64,
) error {
	run, err := s.runs.GetRun(context.Background(), runID)
	if err != nil || run.AppID != appID {
		return errRunNotFound
	}
	appConn.BeginReplay(runID, lastAttempt, lastSeq)
	appConn.Subscribe(runID)
	attempt, seq, err := s.replayWS(appConn, &SubscribeMessage{
		Type: contract.WSMessageSubscribe, RunID: runID,
		LastAttempt: lastAttempt, LastSeq: lastSeq,
	})
	if endErr := appConn.EndReplay(runID, attempt, seq); err == nil {
		err = endErr
	}
	if err != nil {
		return err
	}
	return s.sendWS(appConn, CaughtUpMessage{
		Type: contract.WSMessageCaughtUp, RunID: runID,
		Attempt: attempt, Seq: seq,
	})
}

func (s *Server) replayWS(appConn *connection.AppConn, msg *SubscribeMessage) (int32, int64, error) {
	attempt, seq := msg.LastAttempt, msg.LastSeq
	for {
		events, err := s.runs.ListEvents(context.Background(), msg.RunID, attempt, seq, wsReplayPageSize)
		if err != nil {
			return attempt, seq, err
		}
		for _, event := range events {
			view := EventMessage{
				Type:          contract.WSMessageEvent,
				RunID:         event.RunID,
				Attempt:       int32(event.Attempt),
				Seq:           event.Seq,
				Source:        event.Source,
				EventType:     event.Type,
				EventName:     event.Name,
				SchemaVersion: event.SchemaVersion,
				Data:          event.Payload,
				TSMillis:      event.TSMs,
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
