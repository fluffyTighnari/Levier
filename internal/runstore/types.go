// Middleware — internal/runstore
//
// RunStore entity types.
//
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"encoding/json"
	"time"
)

// App is a registered application.
type App struct {
	AppID      string    `json:"app_id"`
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
	IdempotencyKey   string          `json:"-"`
	RequestHash      string          `json:"-"`
	Runtime          string          `json:"runtime"`
	Strategy         string          `json:"strategy"`
	Params           json.RawMessage `json:"params"`
	Status           string          `json:"status"`
	Attempt          int             `json:"attempt"`
	SessionID        string          `json:"session_id,omitempty"`
	Model            json.RawMessage `json:"model"`
	RunTimeoutMS     int             `json:"run_timeout_ms"`
	TraceID          string          `json:"trace_id"`
	AssignedDaemonID string          `json:"assigned_daemon_id,omitempty"`
	LeaseEpoch       int64           `json:"lease_epoch"`
	AssignedAt       *time.Time      `json:"assigned_at,omitempty"`
	LastEventAttempt int             `json:"last_event_attempt"`
	LastEventSeq     int64           `json:"last_event_seq"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Event is one persisted run event.
type Event struct {
	RunID   string          `json:"run_id"`
	Attempt int             `json:"attempt"`
	Seq     int64           `json:"seq"`
	Source  string          `json:"source"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	TSMs    int64           `json:"ts_ms"`
}
