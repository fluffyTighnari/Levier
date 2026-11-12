// Middleware — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"net/http"
	"strings"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

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
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), err.Error(), false)
		return
	}

	params, err := json.Marshal(emptyMap(request.Strategy.Params))
	if err != nil {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "invalid strategy params", false)
		return
	}
	input, err := json.Marshal(request.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "invalid input", false)
		return
	}
	model, err := json.Marshal(request.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "invalid model", false)
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
		writeError(w, http.StatusConflict, string(errs.CodeIdempotencyConflict), "idempotency key was reused with a different request", false)
		return
	}
	if errors.Is(err, runstore.ErrInvalidRun) {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), err.Error(), false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "create run failed", true)
		return
	}

	if run.Status == runstore.StatusQueued {
		if s.queue == nil {
			writeError(w, http.StatusServiceUnavailable, string(errs.CodeQueueUnavailable), "dispatch queue is not configured", true)
			return
		}
		if _, err := s.queue.Enqueue(r.Context(), workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
			writeError(w, http.StatusServiceUnavailable, string(errs.CodeQueueUnavailable), "enqueue run task failed", true)
			return
		}
		timeoutAt := run.CreatedAt
		if timeoutAt.IsZero() {
			timeoutAt = time.Now()
		}
		timeoutAt = timeoutAt.Add(time.Duration(runTimeout) * time.Millisecond)
		if timeoutAt.Before(time.Now()) {
			timeoutAt = time.Now()
		}
		if _, err := s.queue.Enqueue(r.Context(), workflow.NewTask(run.RunID, workflow.TaskTimeout, timeoutAt, 0)); err != nil {
			writeError(w, http.StatusServiceUnavailable, string(errs.CodeQueueUnavailable), "enqueue run timeout task failed", true)
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
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
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
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "after_attempt must be a non-negative integer", false)
		return
	}
	afterSeq, err := queryInt64(r.URL.Query().Get("after_seq"))
	if err != nil {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "after_seq must be a non-negative integer", false)
		return
	}
	limit, err := queryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "limit must be between 1 and 1000", false)
		return
	}
	run, err := s.runs.GetRun(r.Context(), runID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	}
	events, err := s.runs.ListEvents(r.Context(), runID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "list events failed", true)
		return
	}
	writeJSON(w, http.StatusOK, eventListResponse(events, limit))
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	}
	if s.canceller == nil {
		writeError(w, http.StatusNotImplemented, string(errs.CodeNotImplemented), "cancellation is not configured", false)
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
		writeError(w, http.StatusNotFound, string(errs.CodeRunNotFound), "run not found", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "cancel run failed", true)
		return
	}
	writeJSON(w, http.StatusOK, CancelRunResponse{
		RunID:  cancelled.RunID,
		Status: RunStatus(cancelled.Status),
	})
}
