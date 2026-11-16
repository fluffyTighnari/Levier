// Middleware — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/levier/internal/errs"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/register"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

const defaultMaxBodyBytes = 1 << 20

// RunStore is the persistence subset used by the HTTP API.
type RunStore interface {
	CreateRun(ctx context.Context, run runstore.Run) (runstore.Run, error)
	GetRun(ctx context.Context, runID string) (runstore.Run, error)
	ListEvents(ctx context.Context, runID string, afterAttempt int32, afterSeq int64, limit int) ([]runstore.Event, error)
}

// SessionStore is the optional longterm session subset used by the HTTP API.
type SessionStore interface {
	GetSession(ctx context.Context, sessionID string) (runstore.Session, error)
	ListSessionEvents(ctx context.Context, sessionID string, afterAttempt int32, afterSeq int64, limit int) ([]runstore.Event, error)
}

// AppRegistry is the App registration subset used by the HTTP API.
type AppRegistry interface {
	RegisterApp(ctx context.Context, name string, multiagent bool) (runstore.App, string, bool, error)
	GetApp(ctx context.Context, appID string) (runstore.App, error)
	AuthenticateAPIKey(ctx context.Context, key string) (runstore.App, error)
	UpdateApp(ctx context.Context, appID, name string, multiagent bool) (runstore.App, error)
	RotateAPIKey(ctx context.Context, appID string) (string, error)
}

// CatalogRegistry is the optional App tool/MCP catalog subset.
// Canceller cancels one non-terminal run.
type Canceller interface {
	CancelRun(ctx context.Context, runID, reason string) (runstore.Run, error)
}

type CatalogRegistry interface {
	ReplaceLocalTools(ctx context.Context, appID string, tools []register.ToolInput) ([]register.ToolMaterial, error)
	SetAppMCPRefs(ctx context.Context, appID string, names []string) error
}

// TaskQueue is the dispatch queue subset used by the HTTP API.
type TaskQueue interface {
	Enqueue(ctx context.Context, task workflow.Task) (workflow.Task, error)
}

// Config controls the minimal HTTP API.
type Config struct {
	BearerToken          string
	AdminToken           string
	AllowLegacyBearer    bool
	DefaultRunTimeoutMS  int
	DefaultToolTimeoutMS int
	MaxBodyBytes         int64
	WSTokenTTL           time.Duration
	AppHub               *connection.AppHub
	ReadinessCheck       func(context.Context) error
	Canceller            Canceller
	Metrics              *observe.Registry
	MetricsCollector     func(context.Context)
	Logger               *slog.Logger
}

// Server implements the minimal App HTTP API.
type Server struct {
	runs      RunStore
	queue     TaskQueue
	apps      AppRegistry
	sessions  SessionStore
	appHub    *connection.AppHub
	wsTokens  *WSTokenStore
	canceller Canceller
	cfg       Config
	log       *slog.Logger
	mux       *http.ServeMux
}

// NewServer builds the HTTP handler.
func NewServer(runs RunStore, queue TaskQueue, cfg Config, registries ...AppRegistry) *Server {
	if cfg.DefaultRunTimeoutMS <= 0 {
		cfg.DefaultRunTimeoutMS = 1_800_000
	}
	if cfg.DefaultToolTimeoutMS <= 0 {
		cfg.DefaultToolTimeoutMS = 60_000
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	appHub := cfg.AppHub
	if appHub == nil {
		appHub = connection.NewAppHub()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	server := &Server{
		runs:      runs,
		queue:     queue,
		cfg:       cfg,
		log:       cfg.Logger,
		appHub:    appHub,
		wsTokens:  NewWSTokenStore(cfg.WSTokenTTL),
		canceller: cfg.Canceller,
		mux:       http.NewServeMux(),
	}
	if len(registries) > 0 {
		server.apps = registries[0]
	}
	if sessions, ok := runs.(SessionStore); ok {
		server.sessions = sessions
	}
	server.routes()
	return server
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return requestIDMiddleware(s.instrumentHandler(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	if s.cfg.Metrics != nil {
		s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	}
	s.mux.HandleFunc("POST "+APIPrefix+"/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET "+APIPrefix+"/runs/{run_id}", s.handleGetRun)
	s.mux.HandleFunc("GET "+APIPrefix+"/runs/{run_id}/events", s.handleListEvents)
	s.mux.HandleFunc("POST "+APIPrefix+"/runs/{run_id}/cancel", s.handleCancelRun)
	s.mux.HandleFunc("GET "+APIPrefix+"/sessions/{session_id}", s.handleGetSession)
	s.mux.HandleFunc("GET "+APIPrefix+"/sessions/{session_id}/events", s.handleListSessionEvents)
	s.mux.HandleFunc("POST "+APIPrefix+"/ws-token", s.handleIssueWSToken)
	s.mux.HandleFunc("POST /admin/apps", s.handleAdminCreateApp)
	s.mux.HandleFunc("POST /admin/mcp", s.handleAdminUpsertMCP)
	s.mux.HandleFunc("GET /admin/mcp", s.handleAdminListMCP)
	s.mux.HandleFunc("DELETE /admin/mcp/{name}", s.handleAdminDeleteMCP)
	s.mux.HandleFunc("POST /admin/apps/{app_id}/rotate-key", s.handleAdminRotateKey)
	s.mux.HandleFunc("POST "+APIPrefix+"/apps", s.handleUpdateApp)
	s.mux.HandleFunc("GET "+APIPrefix+"/apps/{app_id}", s.handleGetApp)
	s.mux.Handle(APIPrefix+"/ws", s.websocketHandler(s.handleWS))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadinessCheck == nil {
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{
			Status: "degraded", Database: "not_configured",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.cfg.ReadinessCheck(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{
			Status: "degraded", Database: "unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ready", Database: "up"})
}

func (s *Server) readJSONBody(w http.ResponseWriter, r *http.Request, target any, required bool) ([]byte, bool) {
	if r.Body == nil {
		if required {
			writeErrorCode(w, errs.CodeBadRequest, "request body is required", false)
			return nil, false
		}
		return nil, true
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeErrorCode(w, errs.CodeRequestTooLarge, "request body is too large", false)
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if required {
			writeErrorCode(w, errs.CodeBadRequest, "request body is required", false)
			return nil, false
		}
		return body, true
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "invalid JSON body", false)
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErrorCode(w, errs.CodeBadRequest, "invalid JSON body", false)
		return nil, false
	}
	return body, true
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeErrorCode derives the HTTP status from the shared error model.
func writeErrorCode(w http.ResponseWriter, code errs.Code, message string, retryable bool) {
	writeError(w, errs.HTTPStatus(code), string(code), message, retryable)
}

func writeError(w http.ResponseWriter, statusCode int, code, message string, retryable bool) {
	writeJSON(w, statusCode, ErrorResponse{Error: ErrorBody{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		RequestID: w.Header().Get(RequestIDHeader),
	}})
}

func emptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func requestHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func queryInt32(value string) (int32, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid int32")
	}
	return int32(parsed), nil
}

func queryInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid int64")
	}
	return parsed, nil
}

func queryLimit(value string) (int, error) {
	if value == "" {
		return 100, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 1000 {
		return 0, fmt.Errorf("invalid limit")
	}
	return parsed, nil
}
