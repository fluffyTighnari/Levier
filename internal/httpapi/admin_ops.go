// Levier — operator-facing HTTP surface.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/errs"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

// AdminRunView is the operator projection of one run.
type AdminRunView struct {
	RunID            string          `json:"run_id"`
	AppID            string          `json:"app_id"`
	Runtime          string          `json:"runtime"`
	RuntimeName      string          `json:"runtime_name"`
	Status           string          `json:"status"`
	Attempt          int             `json:"attempt"`
	SessionID        string          `json:"session_id,omitempty"`
	TraceID          string          `json:"trace_id,omitempty"`
	AssignedDaemonID string          `json:"assigned_daemon_id,omitempty"`
	LeaseEpoch       int64           `json:"lease_epoch"`
	AssignedAt       *time.Time      `json:"assigned_at,omitempty"`
	LastEventAttempt int             `json:"last_event_attempt"`
	LastEventSeq     int64           `json:"last_event_seq"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Tasks            []AdminTaskView `json:"tasks,omitempty"`
}

// AdminRunListResponse is a cursor-paginated operator run list.
type AdminRunListResponse struct {
	Runs       []AdminRunView `json:"runs"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// AdminTaskView is the operator projection of one workflow task.
type AdminTaskView struct {
	TaskID         string     `json:"task_id"`
	RunID          string     `json:"run_id"`
	TaskType       string     `json:"task_type"`
	Status         string     `json:"status"`
	Attempt        int        `json:"attempt"`
	MaxAttempts    int        `json:"max_attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LeaseOwner     string     `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AdminTaskListResponse is the operator task list.
type AdminTaskListResponse struct {
	Tasks []AdminTaskView `json:"tasks"`
}

// AdminDaemonView is the operator projection of a connected daemon.
type AdminDaemonView struct {
	DaemonID          string    `json:"daemon_id"`
	RuntimeName       string    `json:"runtime_name"`
	Status            string    `json:"status"`
	RuntimeType       string    `json:"runtime_type"`
	MaxConcurrentRuns int32     `json:"max_concurrent_runs"`
	Running           int32     `json:"running"`
	Reserved          int32     `json:"reserved"`
	Available         int32     `json:"available"`
	Healthy           bool      `json:"healthy"`
	ConnectedAt       time.Time `json:"connected_at"`
	LastHeartbeat     time.Time `json:"last_heartbeat"`
}

// AdminDaemonListResponse is the operator daemon list.
type AdminDaemonListResponse struct {
	Daemons []AdminDaemonView `json:"daemons"`
}

func (s *Server) handleAdminListRuns(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	lister, ok := s.runs.(RunLister)
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "run listing is not configured", false)
		return
	}
	statuses, err := parseRunStatuses(r.URL.Query().Get("status"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	limit, err := queryAdminLimit(r.URL.Query().Get("limit"), 50, 200)
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	page, err := lister.ListRuns(r.Context(), runstore.RunFilter{
		Statuses:  statuses,
		AppID:     r.URL.Query().Get("app_id"),
		DaemonID:  r.URL.Query().Get("daemon_id"),
		SessionID: r.URL.Query().Get("session_id"),
		Limit:     limit,
		Cursor:    r.URL.Query().Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, runstore.ErrInvalidRun) {
			writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		} else {
			writeErrorCode(w, errs.CodeInternal, "list runs failed", true)
		}
		return
	}
	views := make([]AdminRunView, 0, len(page.Runs))
	for _, run := range page.Runs {
		views = append(views, adminRunView(run, nil))
	}
	writeJSON(w, http.StatusOK, AdminRunListResponse{Runs: views, NextCursor: page.NextCursor})
}

func (s *Server) handleAdminGetRun(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get run failed", true)
		return
	}
	tasks := []workflow.Task(nil)
	if lister, ok := s.queue.(TaskLister); ok {
		tasks, err = lister.ListTasks(r.Context(), workflow.TaskFilter{
			RunID: run.RunID,
			Limit: 100,
		})
		if err != nil {
			writeErrorCode(w, errs.CodeInternal, "list run tasks failed", true)
			return
		}
	}
	writeJSON(w, http.StatusOK, adminRunView(run, tasks))
}

func (s *Server) handleAdminCancelRun(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	run, err := s.runs.GetRun(r.Context(), r.PathValue("run_id"))
	if errors.Is(err, runstore.ErrRunNotFound) {
		writeErrorCode(w, errs.CodeRunNotFound, "run not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get run failed", true)
		return
	}
	if s.canceller == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "cancellation is not configured", false)
		return
	}
	reason := "operator"
	var request CancelRunRequest
	if _, ok := s.readJSONBody(w, r, &request, false); !ok {
		return
	}
	if strings.TrimSpace(request.Reason) != "" {
		reason = strings.TrimSpace(request.Reason)
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
	s.log.Info("admin action",
		"action", "run.cancel",
		"run_id", run.RunID,
		"app_id", run.AppID,
		"assigned_daemon_id", run.AssignedDaemonID,
		"reason", reason,
		"previous_status", run.Status,
		"result_status", cancelled.Status,
		"request_id", r.Header.Get("X-Request-ID"),
	)
	writeJSON(w, http.StatusOK, CancelRunResponse{
		RunID:  cancelled.RunID,
		Status: RunStatus(cancelled.Status),
	})
}

func (s *Server) handleAdminListDaemons(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	if s.cfg.Daemons == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "daemon inventory is not configured", false)
		return
	}
	infos := s.cfg.Daemons.List()
	daemons := make([]AdminDaemonView, 0, len(infos))
	for _, info := range infos {
		used := info.RunningCount
		if info.Reserved > used {
			used = info.Reserved
		}
		total := info.MaxConcurrentRuns
		if total <= 0 {
			total = 1
		}
		available := total - used
		if available < 0 {
			available = 0
		}
		daemons = append(daemons, AdminDaemonView{
			DaemonID:          info.DaemonID,
			RuntimeName:       info.RuntimeName,
			Status:            info.Status,
			RuntimeType:       info.RuntimeType.String(),
			MaxConcurrentRuns: total,
			Running:           info.RunningCount,
			Reserved:          info.Reserved,
			Available:         available,
			Healthy:           info.Healthy,
			ConnectedAt:       info.ConnectedAt,
			LastHeartbeat:     info.LastHeartbeat,
		})
	}
	writeJSON(w, http.StatusOK, AdminDaemonListResponse{Daemons: daemons})
}

func (s *Server) handleAdminListTasks(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	lister, ok := s.queue.(TaskLister)
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "task listing is not configured", false)
		return
	}
	statuses, err := parseTaskStatuses(r.URL.Query().Get("status"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	taskTypes, err := parseTaskTypes(r.URL.Query().Get("type"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	limit, err := queryAdminLimit(r.URL.Query().Get("limit"), 100, 500)
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	offset, err := queryAdminOffset(r.URL.Query().Get("offset"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	tasks, err := lister.ListTasks(r.Context(), workflow.TaskFilter{
		Statuses: statuses,
		Types:    taskTypes,
		RunID:    r.URL.Query().Get("run_id"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		if errors.Is(err, workflow.ErrInvalidTask) {
			writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		} else {
			writeErrorCode(w, errs.CodeInternal, "list tasks failed", true)
		}
		return
	}
	views := make([]AdminTaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, adminTaskView(task))
	}
	writeJSON(w, http.StatusOK, AdminTaskListResponse{Tasks: views})
}

func adminRunView(run runstore.Run, tasks []workflow.Task) AdminRunView {
	view := AdminRunView{
		RunID:            run.RunID,
		AppID:            run.AppID,
		Runtime:          run.Runtime,
		RuntimeName:      run.RuntimeName,
		Status:           run.Status,
		Attempt:          run.Attempt,
		SessionID:        run.SessionID,
		TraceID:          run.TraceID,
		AssignedDaemonID: run.AssignedDaemonID,
		LeaseEpoch:       run.LeaseEpoch,
		AssignedAt:       run.AssignedAt,
		LastEventAttempt: run.LastEventAttempt,
		LastEventSeq:     run.LastEventSeq,
		CreatedAt:        run.CreatedAt,
		UpdatedAt:        run.UpdatedAt,
	}
	for _, task := range tasks {
		view.Tasks = append(view.Tasks, adminTaskView(task))
	}
	return view
}

func adminTaskView(task workflow.Task) AdminTaskView {
	view := AdminTaskView{
		TaskID:      task.ID,
		RunID:       task.RunID,
		TaskType:    string(task.Type),
		Status:      string(task.Status),
		Attempt:     task.Attempt,
		MaxAttempts: task.MaxAttempts,
		AvailableAt: task.AvailableAt,
		LeaseOwner:  task.LeaseOwner,
		LastError:   task.LastError,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
	if !task.LeaseExpiresAt.IsZero() {
		expires := task.LeaseExpiresAt
		view.LeaseExpiresAt = &expires
	}
	return view
}

func parseRunStatuses(value string) ([]string, error) {
	values := splitCSV(value)
	for index, status := range values {
		status = strings.ToLower(status)
		if !runstore.ValidRunStatus(status) {
			return nil, fmt.Errorf("invalid run status %q", status)
		}
		values[index] = status
	}
	return values, nil
}

func parseTaskStatuses(value string) ([]workflow.Status, error) {
	values := splitCSV(value)
	result := make([]workflow.Status, 0, len(values))
	for _, value := range values {
		status := workflow.Status(strings.ToUpper(value))
		if !status.Valid() {
			return nil, fmt.Errorf("invalid task status %q", value)
		}
		result = append(result, status)
	}
	return result, nil
}

func parseTaskTypes(value string) ([]workflow.TaskType, error) {
	values := splitCSV(value)
	result := make([]workflow.TaskType, 0, len(values))
	for _, value := range values {
		taskType := workflow.TaskType(strings.ToUpper(value))
		if !taskType.Valid() {
			return nil, fmt.Errorf("invalid task type %q", value)
		}
		result = append(result, taskType)
	}
	return result, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func queryAdminLimit(value string, fallback, max int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > max {
		return 0, fmt.Errorf("limit must be between 1 and %d", max)
	}
	return parsed, nil
}

func queryAdminOffset(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("offset must be non-negative")
	}
	return parsed, nil
}
