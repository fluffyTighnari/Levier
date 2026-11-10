// Middleware — internal/httpapi
//
// Minimal App HTTP API: create runs, read run status, replay events.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/register"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
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
	s.mux.HandleFunc("POST /admin/apps/{app_id}/rotate-key", s.handleAdminRotateKey)
	s.mux.HandleFunc("POST "+APIPrefix+"/apps", s.handleUpdateApp)
	s.mux.HandleFunc("GET "+APIPrefix+"/apps/{app_id}", s.handleGetApp)
	s.mux.Handle(APIPrefix+"/ws", s.websocketHandler(s.handleWS))
}

func (s *Server) authDisabled() bool {
	return s.cfg.BearerToken == "" && s.cfg.AdminToken == "" && s.apps == nil
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(value, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	}
	return ""
}

func matchesToken(provided, expected string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Server) isAdmin(r *http.Request) bool {
	return matchesToken(bearerToken(r), s.cfg.AdminToken)
}

func (s *Server) legacyAllowed(r *http.Request) bool {
	return s.cfg.AllowLegacyBearer && matchesToken(bearerToken(r), s.cfg.BearerToken)
}

func (s *Server) appFromRequest(r *http.Request) (runstore.App, bool) {
	if s.apps == nil {
		return runstore.App{}, false
	}
	token := bearerToken(r)
	if token == "" {
		return runstore.App{}, false
	}
	app, err := s.apps.AuthenticateAPIKey(r.Context(), token)
	if err != nil {
		return runstore.App{}, false
	}
	return app, true
}

func (s *Server) resolveRequestApp(w http.ResponseWriter, r *http.Request, appID string) (string, bool) {
	if s.authDisabled() {
		if appID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	if s.isAdmin(r) {
		if appID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	if app, ok := s.appFromRequest(r); ok {
		if appID != "" && appID != app.AppID {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "app_id does not match the authenticated app", false)
			return "", false
		}
		return app.AppID, true
	}
	if s.legacyAllowed(r) {
		if appID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token", false)
	return "", false
}

func (s *Server) canAccessApp(r *http.Request, appID string) bool {
	if s.authDisabled() || s.isAdmin(r) || s.legacyAllowed(r) {
		return true
	}
	app, ok := s.appFromRequest(r)
	return ok && app.AppID == appID
}

func (s *Server) handleIssueWSToken(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.resolveRequestApp(w, r, r.URL.Query().Get("app_id"))
	if !ok {
		return
	}
	token := s.wsTokens.Issue(appID)
	if token == "" {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "issue ws token failed", true)
		return
	}
	writeJSON(w, http.StatusOK, WSTokenResponse{
		WSURL:     WSURL,
		WSToken:   token,
		ExpiresIn: int64(s.wsTokens.TTL().Seconds()),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var request CreateRunRequest
	body, ok := s.readJSONBody(w, r, &request, true)
	if !ok {
		return
	}
	appID, ok := s.resolveRequestApp(w, r, request.AppID)
	if !ok {
		return
	}
	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), false)
		return
	}

	params, err := json.Marshal(emptyMap(request.Strategy.Params))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy params", false)
		return
	}
	input, err := json.Marshal(request.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid input", false)
		return
	}
	model, err := json.Marshal(request.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid model", false)
		return
	}

	runTimeout := request.Timeouts.RunMS
	if runTimeout <= 0 {
		runTimeout = int64(s.cfg.DefaultRunTimeoutMS)
	}
	toolTimeout := request.Timeouts.ToolMS
	if toolTimeout <= 0 {
		toolTimeout = int64(s.cfg.DefaultToolTimeoutMS)
	}
	runID := id.New()
	run, err := s.runs.CreateRun(r.Context(), runstore.Run{
		RunID:          runID,
		AppID:          appID,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		RequestHash:    requestHash(body),
		Runtime:        string(request.Runtime),
		Strategy:       request.Strategy.Name,
		Params:         params,
		Input:          input,
		Model:          model,
		Status:         "queued",
		SessionID:      request.SessionID,
		RunTimeoutMS:   int(runTimeout),
		ToolTimeoutMS:  int(toolTimeout),
		TraceID:        traceIDFromRequest(r),
	})
	if errors.Is(err, runstore.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was reused with a different request", false)
		return
	}
	if errors.Is(err, runstore.ErrInvalidRun) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "create run failed", true)
		return
	}

	if run.RunID == runID {
		if s.queue == nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "dispatch queue is not configured", true)
			return
		}
		if _, err := s.queue.Enqueue(r.Context(), workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "enqueue run task failed", true)
			return
		}
		timeoutAt := time.Now().Add(time.Duration(runTimeout) * time.Millisecond)
		if _, err := s.queue.Enqueue(r.Context(), workflow.NewTask(run.RunID, workflow.TaskTimeout, timeoutAt, 0)); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "enqueue run timeout task failed", true)
			return
		}
	}

	writeJSON(w, http.StatusAccepted, CreateRunResponse{
		RunID:   run.RunID,
		Status:  RunStatus(run.Status),
		WSURL:   WSURL,
		WSToken: s.wsTokens.Issue(appID),
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	strategyParams := map[string]any{}
	if len(run.Params) > 0 {
		_ = json.Unmarshal(run.Params, &strategyParams)
	}
	writeJSON(w, http.StatusOK, RunView{
		RunID:     run.RunID,
		AppID:     run.AppID,
		Runtime:   RuntimeType(run.Runtime),
		Strategy:  StrategyRequest{Name: run.Strategy, Params: strategyParams},
		Status:    RunStatus(run.Status),
		Attempt:   int32(run.Attempt),
		SessionID: run.SessionID,
		TraceID:   run.TraceID,
		CreatedAt: run.CreatedAt,
		UpdatedAt: run.UpdatedAt,
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	afterAttempt, err := queryInt32(r.URL.Query().Get("after_attempt"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "after_attempt must be a non-negative integer", false)
		return
	}
	afterSeq, err := queryInt64(r.URL.Query().Get("after_seq"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "after_seq must be a non-negative integer", false)
		return
	}
	limit, err := queryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be between 1 and 1000", false)
		return
	}
	run, err := s.runs.GetRun(r.Context(), runID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	events, err := s.runs.ListEvents(r.Context(), runID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list events failed", true)
		return
	}
	writeJSON(w, http.StatusOK, eventListResponse(events, limit))
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "session store is not configured", false)
		return
	}
	session, err := s.sessions.GetSession(r.Context(), r.PathValue("session_id"))
	if errors.Is(err, runstore.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get session failed", true)
		return
	}
	if !s.canAccessApp(r, session.AppID) {
		writeError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found", false)
		return
	}
	writeJSON(w, http.StatusOK, sessionView(session))
}

func (s *Server) handleListSessionEvents(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "session store is not configured", false)
		return
	}
	afterAttempt, err := queryInt32(r.URL.Query().Get("after_attempt"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "after_attempt must be a non-negative integer", false)
		return
	}
	afterSeq, err := queryInt64(r.URL.Query().Get("after_seq"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "after_seq must be a non-negative integer", false)
		return
	}
	limit, err := queryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be between 1 and 1000", false)
		return
	}
	sessionID := r.PathValue("session_id")
	session, err := s.sessions.GetSession(r.Context(), sessionID)
	if errors.Is(err, runstore.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get session failed", true)
		return
	}
	if !s.canAccessApp(r, session.AppID) {
		writeError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found", false)
		return
	}
	events, err := s.sessions.ListSessionEvents(r.Context(), sessionID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list session events failed", true)
		return
	}
	writeJSON(w, http.StatusOK, eventListResponse(events, limit))
}

func eventListResponse(events []runstore.Event, limit int) EventListResponse {
	views := make([]RunEventView, 0, len(events))
	for _, event := range events {
		views = append(views, RunEventView{
			RunID:     event.RunID,
			Attempt:   int32(event.Attempt),
			Seq:       event.Seq,
			Source:    event.Source,
			EventType: event.Type,
			Data:      event.Payload,
			TSMillis:  event.TSMs,
		})
	}
	response := EventListResponse{
		Events:   views,
		CaughtUp: len(views) < limit,
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		response.NextAttempt = int32(last.Attempt)
		response.NextSeq = last.Seq
	}
	return response
}

func sessionView(session runstore.Session) SessionView {
	return SessionView{
		SessionID:      session.SessionID,
		AppID:          session.AppID,
		Status:         session.Status,
		CurrentRunID:   session.CurrentRunID,
		EventAttempt:   int32(session.EventAttempt),
		EventWatermark: session.EventWatermark,
		CreatedAt:      session.CreatedAt,
		UpdatedAt:      session.UpdatedAt,
		ExpiresAt:      session.ExpiresAt,
	}
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	if s.canceller == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "cancellation is not configured", false)
		return
	}
	reason := "user"
	var request CancelRunRequest
	if _, ok := s.readJSONBody(w, r, &request, false); !ok {
		return
	}
	if request.Reason != "" {
		reason = request.Reason
	}
	cancelled, err := s.canceller.CancelRun(r.Context(), run.RunID, reason)
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "cancel run failed", true)
		return
	}
	writeJSON(w, http.StatusOK, CancelRunResponse{
		RunID:  cancelled.RunID,
		Status: RunStatus(cancelled.Status),
	})
}

func (s *Server) handleAdminCreateApp(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "admin token is required", false)
		return
	}
	if s.apps == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "app registry is not configured", false)
		return
	}
	var request AppUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), false)
		return
	}
	app, key, created, err := s.apps.RegisterApp(r.Context(), request.Name, request.Multiagent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "create app failed", true)
		return
	}
	if created && !s.applyAppCatalog(w, r, app.AppID, request) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, CreateAppResponse{App: appView(app), APIKey: key})
}

func (s *Server) applyAppCatalog(w http.ResponseWriter, r *http.Request, appID string, request AppUpsertRequest) bool {
	catalog, ok := s.apps.(CatalogRegistry)
	if !ok {
		return true
	}
	if request.Tools != nil {
		tools := make([]register.ToolInput, 0, len(request.Tools))
		for _, tool := range request.Tools {
			schema := string(tool.Schema)
			if schema == "" {
				schema = "{}"
			}
			tools = append(tools, register.ToolInput{
				Name:        tool.Name,
				Description: tool.Description,
				SchemaJSON:  schema,
				TimeoutMS:   int(tool.TimeoutMS),
			})
		}
		if _, err := catalog.ReplaceLocalTools(r.Context(), appID, tools); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_TOOL", err.Error(), false)
			return false
		}
	}
	if request.MCPRefs != nil {
		if err := catalog.SetAppMCPRefs(r.Context(), appID, request.MCPRefs); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_MCP_REF", err.Error(), false)
			return false
		}
	}
	return true
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "app API key is required", false)
		return
	}
	var request AppUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), false)
		return
	}
	updated, err := s.apps.UpdateApp(r.Context(), app.AppID, request.Name, request.Multiagent)
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "APP_NAME_CONFLICT", "app name is already in use", false)
		return
	}
	if !s.applyAppCatalog(w, r, app.AppID, request) {
		return
	}
	writeJSON(w, http.StatusOK, appView(updated))
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("app_id")
	if !s.isAdmin(r) {
		app, ok := s.appFromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "app API key is required", false)
			return
		}
		if app.AppID != appID {
			writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found", false)
			return
		}
	}
	if s.apps == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "app registry is not configured", false)
		return
	}
	app, err := s.apps.GetApp(r.Context(), appID)
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get app failed", true)
		return
	}
	writeJSON(w, http.StatusOK, appView(app))
}

func (s *Server) handleAdminRotateKey(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "admin token is required", false)
		return
	}
	if s.apps == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "app registry is not configured", false)
		return
	}
	key, err := s.apps.RotateAPIKey(r.Context(), r.PathValue("app_id"))
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "rotate app key failed", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": key})
}

func appView(app runstore.App) AppView {
	return AppView{
		AppID:      app.AppID,
		Name:       app.Name,
		Multiagent: app.Multiagent,
		Status:     app.Status,
		CreatedAt:  app.CreatedAt,
		UpdatedAt:  app.UpdatedAt,
	}
}

func (s *Server) readJSONBody(w http.ResponseWriter, r *http.Request, target any, required bool) ([]byte, bool) {
	if r.Body == nil {
		if required {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "request body is required", false)
			return nil, false
		}
		return nil, true
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "BAD_REQUEST", "request body is too large", false)
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if required {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "request body is required", false)
			return nil, false
		}
		return body, true
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", false)
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", false)
		return nil, false
	}
	return body, true
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
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
