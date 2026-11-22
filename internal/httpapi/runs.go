// Levier — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fluffyTighnari/levier/internal/errs"
	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

var errQueueUnavailable = errors.New("queue unavailable")

type transactionalRunStore interface {
	CreateRunWithTx(ctx context.Context, run runstore.Run, after runstore.TxFunc) (runstore.Run, error)
}

type transactionalTaskQueue interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, task workflow.Task) (workflow.Task, error)
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
	tenantID := s.tenantForApp(r.Context(), appID)
	if err := request.Validate(); err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	if s.quota != nil {
		if err := s.quota.AllowRun(r.Context(), tenantID, appID); err != nil {
			writeErrorCode(w, errs.CodeQuotaExceeded, err.Error(), false)
			return
		}
	}
	if s.apps != nil {
		setResponseTenant(w, tenantID)
	}
	runtimeType := request.Runtime
	if runtimeType == "" {
		runtimeType = RuntimeNormal
	}

	params, err := json.Marshal(emptyMap(request.Params))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "invalid params", false)
		return
	}
	input, err := json.Marshal(request.Input)
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "invalid input", false)
		return
	}
	var model json.RawMessage = []byte(`{}`)
	if request.Model != nil {
		encoded, err := json.Marshal(request.Model)
		if err != nil {
			writeErrorCode(w, errs.CodeBadRequest, "invalid model", false)
			return
		}
		model = encoded
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
	run, transactional, err := s.createRun(r.Context(), runstore.Run{
		RunID:          runID,
		AppID:          appID,
		TenantID:       tenantID,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		RequestHash:    requestHash(body),
		Runtime:        string(runtimeType),
		RuntimeName:    request.RuntimeName,
		Params:         params,
		Input:          input,
		Model:          model,
		Status:         "queued",
		SessionID:      request.SessionID,
		RunTimeoutMS:   int(runTimeout),
		ToolTimeoutMS:  int(toolTimeout),
		TraceID:        traceIDFromRequest(r),
	}, runTimeout)
	if errors.Is(err, errQueueUnavailable) {
		writeErrorCode(w, errs.CodeQueueUnavailable, "enqueue run task failed", true)
		return
	}
	if errors.Is(err, runstore.ErrIdempotencyConflict) {
		writeErrorCode(w, errs.CodeIdempotencyConflict, "idempotency key was reused with a different request", false)
		return
	}
	if errors.Is(err, runstore.ErrInvalidRun) {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "create run failed", true)
		return
	}

	if !transactional && run.Status == runstore.StatusQueued {
		if s.queue == nil {
			writeErrorCode(w, errs.CodeQueueUnavailable, "dispatch queue is not configured", true)
			return
		}
		if _, err := s.queue.Enqueue(r.Context(), workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
			writeErrorCode(w, errs.CodeQueueUnavailable, "enqueue run task failed", true)
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
			writeErrorCode(w, errs.CodeQueueUnavailable, "enqueue run timeout task failed", true)
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

func (s *Server) createRun(
	ctx context.Context,
	run runstore.Run,
	runTimeout int64,
) (runstore.Run, bool, error) {
	txRuns, txRunsOK := s.runs.(transactionalRunStore)
	txQueue, txQueueOK := s.queue.(transactionalTaskQueue)
	if !txRunsOK || !txQueueOK {
		created, err := s.runs.CreateRun(ctx, run)
		return created, false, err
	}

	created, err := txRuns.CreateRunWithTx(ctx, run, func(ctx context.Context, tx pgx.Tx, created runstore.Run) error {
		if created.Status != runstore.StatusQueued {
			return nil
		}
		dispatch := workflow.NewTask(created.RunID, workflow.TaskDispatch, time.Now(), 0)
		if _, err := txQueue.EnqueueTx(ctx, tx, dispatch); err != nil {
			return fmt.Errorf("%w: enqueue dispatch: %v", errQueueUnavailable, err)
		}
		timeoutAt := created.CreatedAt
		if timeoutAt.IsZero() {
			timeoutAt = time.Now()
		}
		timeoutAt = timeoutAt.Add(time.Duration(runTimeout) * time.Millisecond)
		if timeoutAt.Before(time.Now()) {
			timeoutAt = time.Now()
		}
		timeout := workflow.NewTask(created.RunID, workflow.TaskTimeout, timeoutAt, 0)
		if _, err := txQueue.EnqueueTx(ctx, tx, timeout); err != nil {
			return fmt.Errorf("%w: enqueue timeout: %v", errQueueUnavailable, err)
		}
		return nil
	})
	return created, true, err
}

func (s *Server) tenantForApp(ctx context.Context, appID string) string {
	if s.apps == nil || strings.TrimSpace(appID) == "" {
		return runstore.DefaultTenantID
	}
	app, err := s.apps.GetApp(ctx, appID)
	if err != nil || app.TenantID == "" {
		return runstore.DefaultTenantID
	}
	return app.TenantID
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	setResponseTenant(w, run.TenantID)
	writeJSON(w, http.StatusOK, RunView{
		RunID:       run.RunID,
		AppID:       run.AppID,
		RuntimeName: run.RuntimeName,
		Status:      RunStatus(run.Status),
		Attempt:     int32(run.Attempt),
		SessionID:   run.SessionID,
		TraceID:     run.TraceID,
		CreatedAt:   run.CreatedAt,
		UpdatedAt:   run.UpdatedAt,
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	afterAttempt, err := queryInt32(r.URL.Query().Get("after_attempt"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "after_attempt must be a non-negative integer", false)
		return
	}
	afterSeq, err := queryInt64(r.URL.Query().Get("after_seq"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "after_seq must be a non-negative integer", false)
		return
	}
	limit, err := queryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "limit must be between 1 and 1000", false)
		return
	}
	run, err := s.runs.GetRun(r.Context(), runID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	} else if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	setResponseTenant(w, run.TenantID)
	events, err := s.runs.ListEvents(r.Context(), runID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "list events failed", true)
		return
	}
	writeJSON(w, http.StatusOK, eventListResponse(events, limit))
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get run failed", true)
		return
	}
	if !s.canAccessApp(r, run.AppID) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	setResponseTenant(w, run.TenantID)
	if s.canceller == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "cancellation is not configured", false)
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
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "cancel run failed", true)
		return
	}
	writeJSON(w, http.StatusOK, CancelRunResponse{
		RunID:  cancelled.RunID,
		Status: RunStatus(cancelled.Status),
	})
}
