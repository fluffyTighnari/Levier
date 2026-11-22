// Levier — internal/runstore
//
// RunStore entity types.
//
// Copyright 2026 The Levier Authors.
package runstore

import (
	"encoding/json"
	"time"
)

// DefaultTenantID is the tenant used by local and legacy data.
const DefaultTenantID = "default"

// App is a registered application.
type App struct {
	AppID      string    `json:"app_id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	APIKeyHash string    `json:"-"`
	Multiagent bool      `json:"multiagent"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Run is the authoritative state of one run.
type Run struct {
	RunID            string          `json:"run_id"`
	AppID            string          `json:"app_id"`
	TenantID         string          `json:"tenant_id"`
	IdempotencyKey   string          `json:"-"`
	RequestHash      string          `json:"-"`
	Runtime          string          `json:"runtime"`
	RuntimeName      string          `json:"runtime_name"`
	Params           json.RawMessage `json:"params"`
	Input            json.RawMessage `json:"input"`
	Status           string          `json:"status"`
	Attempt          int             `json:"attempt"`
	SessionID        string          `json:"session_id,omitempty"`
	Model            json.RawMessage `json:"model"`
	RunTimeoutMS     int             `json:"run_timeout_ms"`
	ToolTimeoutMS    int             `json:"tool_timeout_ms"`
	TraceID          string          `json:"trace_id"`
	AssignedDaemonID string          `json:"assigned_daemon_id,omitempty"`
	LeaseEpoch       int64           `json:"lease_epoch"`
	AssignedAt       *time.Time      `json:"assigned_at,omitempty"`
	LastEventAttempt int             `json:"last_event_attempt"`
	LastEventSeq     int64           `json:"last_event_seq"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Session is the App-visible longterm session state.
type Session struct {
	SessionID      string     `json:"session_id"`
	AppID          string     `json:"app_id"`
	TenantID       string     `json:"tenant_id"`
	Status         string     `json:"status"`
	CurrentRunID   string     `json:"current_run_id,omitempty"`
	EventAttempt   int        `json:"event_attempt"`
	EventWatermark int64      `json:"event_watermark"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// Event is one run event submitted to the store.
type Event struct {
	RunID      string          `json:"run_id"`
	Attempt    int             `json:"attempt"`
	Seq        int64           `json:"seq"`
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	TSMs       int64           `json:"ts_ms"`
	LeaseEpoch int64           `json:"lease_epoch"`
	Ephemeral  bool            `json:"ephemeral"`
}

// AppendResult describes one event insertion attempt.
type AppendResult struct {
	Inserted bool
	Stale    bool
	Gap      bool
	GapSize  int64
}
