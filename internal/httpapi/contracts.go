// Levier — internal/httpapi
//
// App-facing HTTP contract types.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"encoding/json"
	"time"
)

const (
	// APIPrefix is the versioned App HTTP prefix.
	APIPrefix = "/api/v1"
	// WSURL is the App WebSocket endpoint.
	WSURL = APIPrefix + "/ws"
)

// RuntimeType is the run placement type.
type RuntimeType string

const (
	// RuntimeNormal uses a shared long-lived harness.
	RuntimeNormal RuntimeType = "normal"
	// RuntimeSandbox uses an isolated harness pod.
	RuntimeSandbox RuntimeType = "sandbox"
	// RuntimeLongterm uses session checkpoint support.
	RuntimeLongterm RuntimeType = "longterm"
)

// RunStatus is the App-visible run state.
type RunStatus string

const (
	// RunStatusQueued is waiting for dispatch.
	RunStatusQueued RunStatus = "queued"
	// RunStatusAssigned is assigned to a harness.
	RunStatusAssigned RunStatus = "assigned"
	// RunStatusRunning is executing.
	RunStatusRunning RunStatus = "running"
	// RunStatusWaiting is paused with a resumable checkpoint.
	RunStatusWaiting RunStatus = "waiting"
	// RunStatusCompleted finished successfully.
	RunStatusCompleted RunStatus = "completed"
	// RunStatusFailed finished with an error.
	RunStatusFailed RunStatus = "failed"
	// RunStatusCancelled was cancelled.
	RunStatusCancelled RunStatus = "cancelled"
	// RunStatusTimeout exceeded its deadline.
	RunStatusTimeout RunStatus = "timeout"
)

// ChatMessage is one conversation message.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// ModelRequest selects a model provider and model.
type ModelRequest struct {
	Provider string            `json:"provider"`
	Model    string            `json:"model"`
	Params   map[string]string `json:"params,omitempty"`
}

// TimeoutRequest carries run and tool deadlines in milliseconds.
type TimeoutRequest struct {
	RunMS  int64 `json:"run_ms,omitempty"`
	ToolMS int64 `json:"tool_ms,omitempty"`
}

// CreateRunRequest is the POST /runs body.
type CreateRunRequest struct {
	AppID       string `json:"app_id,omitempty"`
	RuntimeName string `json:"runtime_name"`
	// Runtime is an optional compatibility field. New App callers should only
	// choose RuntimeName; the control plane defaults to normal.
	Runtime   RuntimeType    `json:"runtime,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	Input     []ChatMessage  `json:"input"`
	SessionID string         `json:"session_id,omitempty"`
	Model     *ModelRequest  `json:"model,omitempty"`
	Timeouts  TimeoutRequest `json:"timeouts,omitempty"`
}

// CreateRunResponse is returned after accepting a run.
type CreateRunResponse struct {
	RunID     string    `json:"run_id"`
	Status    RunStatus `json:"status"`
	SessionID string    `json:"session_id,omitempty"`
	WSURL     string    `json:"ws_url"`
	WSToken   string    `json:"ws_token"`
}

// RunView is the App-visible run projection.
type RunView struct {
	RunID       string    `json:"run_id"`
	AppID       string    `json:"app_id"`
	RuntimeName string    `json:"runtime_name"`
	Status      RunStatus `json:"status"`
	Attempt     int32     `json:"attempt"`
	SessionID   string    `json:"session_id,omitempty"`
	TraceID     string    `json:"trace_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RunEventView is one persisted run event.
type RunEventView struct {
	RunID         string          `json:"run_id"`
	Attempt       int32           `json:"attempt"`
	Seq           int64           `json:"seq"`
	Source        string          `json:"source"`
	EventType     string          `json:"event_type"`
	Name          string          `json:"name,omitempty"`
	SchemaVersion string          `json:"schema_version,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	TSMillis      int64           `json:"ts_ms"`
}

// EventListResponse is the events replay response.
type EventListResponse struct {
	Events      []RunEventView `json:"events"`
	CaughtUp    bool           `json:"caught_up"`
	NextAttempt int32          `json:"next_attempt,omitempty"`
	NextSeq     int64          `json:"next_seq,omitempty"`
}

// WSTokenResponse is returned when an App requests a fresh WebSocket token.
type WSTokenResponse struct {
	WSURL     string `json:"ws_url"`
	WSToken   string `json:"ws_token"`
	ExpiresIn int64  `json:"expires_in"`
}

// CancelRunRequest is the optional POST /runs/{run_id}/cancel body.
type CancelRunRequest struct {
	Reason string `json:"reason,omitempty"`
}

// CancelRunResponse is returned after a cancellation request.
type CancelRunResponse struct {
	RunID  string    `json:"run_id"`
	Status RunStatus `json:"status"`
}

// ResumeRunResponse is returned after a waiting run is requeued.
type ResumeRunResponse struct {
	RunID     string    `json:"run_id"`
	SessionID string    `json:"session_id,omitempty"`
	Status    RunStatus `json:"status"`
}

// SessionView is the App-visible longterm session.
type SessionView struct {
	SessionID      string     `json:"session_id"`
	AppID          string     `json:"app_id"`
	TenantID       string     `json:"tenant_id"`
	Status         string     `json:"status"`
	CurrentRunID   string     `json:"current_run_id,omitempty"`
	EventAttempt   int32      `json:"event_attempt"`
	EventWatermark int64      `json:"event_watermark"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// AppView is the App registration projection.
type AppView struct {
	AppID      string    `json:"app_id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	Multiagent bool      `json:"multiagent"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ToolDefinition is one local tool declared by an App.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	TimeoutMS   int64           `json:"timeout_ms,omitempty"`
}

// AppUpsertRequest is the POST /admin/apps and POST /apps body.
type AppUpsertRequest struct {
	TenantID   string           `json:"tenant_id,omitempty"`
	Name       string           `json:"name"`
	Multiagent bool             `json:"multiagent"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	MCPRefs    []string         `json:"mcp_refs,omitempty"`
}

// CreateAppResponse returns the one-time App API key.
type CreateAppResponse struct {
	App    AppView `json:"app"`
	APIKey string  `json:"api_key"`
}

// AppListResponse returns admin-visible App metadata without key material.
type AppListResponse struct {
	Apps []AppView `json:"apps"`
}

// ErrorBody is the structured error payload.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorResponse is the standard HTTP error envelope.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// HealthResponse is the liveness payload.
type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database,omitempty"`
}
