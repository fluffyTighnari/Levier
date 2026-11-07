// Middleware — internal/httpapi
//
// Minimal App HTTP API: create runs, read run status, replay events.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
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

// TaskQueue is the dispatch queue subset used by the HTTP API.
type TaskQueue interface {
	Enqueue(ctx context.Context, task workflow.Task) (workflow.Task, error)
}

// Config controls the minimal HTTP API.
type Config struct {
	BearerToken          string
	DefaultRunTimeoutMS  int
	DefaultToolTimeoutMS int
	MaxBodyBytes         int64
}

// Server implements the minimal App HTTP API.
type Server struct {
	runs  RunStore
	queue TaskQueue
	cfg   Config
	mux   *http.ServeMux
}

// NewServer builds the HTTP handler.
func NewServer(runs RunStore, queue TaskQueue, cfg Config) *Server {
	if cfg.DefaultRunTimeoutMS <= 0 {
		cfg.DefaultRunTimeoutMS = 1_800_000
	}
	if cfg.DefaultToolTimeoutMS <= 0 {
		cfg.DefaultToolTimeoutMS = 60_000
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	server := &Server{runs: runs, queue: queue, cfg: cfg, mux: http.NewServeMux()}
	server.routes()
	return server
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST "+APIPrefix+"/runs", s.auth(s.handleCreateRun))
	s.mux.HandleFunc("GET "+APIPrefix+"/runs/{run_id}", s.auth(s.handleGetRun))
	s.mux.HandleFunc("GET "+APIPrefix+"/runs/{run_id}/events", s.auth(s.handleListEvents))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "BAD_REQUEST", "request body is too large", false)
		return
	}
	var request CreateRunRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", false)
		return
	}
	if request.AppID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "app_id is required", false)
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
		AppID:          request.AppID,
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
		TraceID:        id.New(),
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
	}

	writeJSON(w, http.StatusAccepted, CreateRunResponse{
		RunID:  run.RunID,
		Status: RunStatus(run.Status),
		WSURL:  WSURL,
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
	if _, err := s.runs.GetRun(r.Context(), runID); errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", false)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "get run failed", true)
		return
	}
	events, err := s.runs.ListEvents(r.Context(), runID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list events failed", true)
		return
	}
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
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	if s.cfg.BearerToken == "" {
		return next
	}
	expected := "Bearer " + s.cfg.BearerToken
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token", false)
			return
		}
		next(w, r)
	}
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
