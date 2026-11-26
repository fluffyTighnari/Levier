// Package cluster owns cross-replica connection registry and lease state.
//
// PostgreSQL is the source of truth. The package stores gateway instances,
// Runtime connections, App connections, App execution leases, and the durable
// notification outbox used to wake other replicas.
//
// Copyright 2026 The Levier Authors.
package cluster

import (
	"encoding/json"
	"time"
)

const (
	// StatusOnline marks a live lease holder.
	StatusOnline = "online"
	// StatusOffline marks an expired or closed lease holder.
	StatusOffline = "offline"
)

const (
	// RoleControl runs scheduling and workflow workers.
	RoleControl = "control"
	// RoleAPI serves the public HTTP API.
	RoleAPI = "api"
	// RoleRuntimeGateway owns daemon RuntimeChannel connections.
	RoleRuntimeGateway = "runtime-gateway"
	// RoleAppGateway owns App WebSocket connections and local tools.
	RoleAppGateway = "app-gateway"
)

// GatewayInstance is one control / runtime-gateway / app-gateway process.
type GatewayInstance struct {
	GatewayID       string            `json:"gateway_id"`
	Epoch           int64             `json:"epoch"`
	Roles           []string          `json:"roles"`
	Status          string            `json:"status"`
	Version         string            `json:"version"`
	Address         string            `json:"address"`
	InternalAddress string            `json:"internal_address,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	StartedAt       time.Time         `json:"started_at"`
	LastSeenAt      time.Time         `json:"last_seen_at"`
	LeaseExpiresAt  time.Time         `json:"lease_expires_at"`
}

// RuntimeRunRef is one in-flight Run reported by a Runtime.
type RuntimeRunRef struct {
	RunID      string `json:"run_id"`
	LeaseEpoch int64  `json:"lease_epoch"`
}

// RuntimeConnection is one daemon registered on a Runtime Gateway.
type RuntimeConnection struct {
	DaemonID          string            `json:"daemon_id"`
	GatewayID         string            `json:"gateway_id"`
	ConnectionEpoch   int64             `json:"connection_epoch"`
	RuntimeName       string            `json:"runtime_name"`
	RuntimeType       string            `json:"runtime_type"`
	Status            string            `json:"status"`
	Version           string            `json:"version"`
	MaxConcurrentRuns int               `json:"max_concurrent_runs"`
	RunningCount      int               `json:"running_count"`
	Reserved          int               `json:"reserved"`
	Labels            map[string]string `json:"labels,omitempty"`
	Capabilities      map[string]string `json:"capabilities,omitempty"`
	RunningRuns       []RuntimeRunRef   `json:"running_runs,omitempty"`
	ConnectedAt       time.Time         `json:"connected_at"`
	LastHeartbeatAt   time.Time         `json:"last_heartbeat_at"`
	LeaseExpiresAt    time.Time         `json:"lease_expires_at"`
}

// AppConnection is one App WebSocket connection registered on an App Gateway.
type AppConnection struct {
	ConnectionID    string            `json:"connection_id"`
	AppID           string            `json:"app_id"`
	TenantID        string            `json:"tenant_id"`
	GatewayID       string            `json:"gateway_id"`
	ConnectionEpoch int64             `json:"connection_epoch"`
	ProtocolVersion int               `json:"protocol_version"`
	Capabilities    map[string]string `json:"capabilities,omitempty"`
	Status          string            `json:"status"`
	ConnectedAt     time.Time         `json:"connected_at"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	LeaseExpiresAt  time.Time         `json:"lease_expires_at"`
}

// AppExecutionLease reserves one App connection for local tool execution.
type AppExecutionLease struct {
	AppID        string    `json:"app_id"`
	ConnectionID string    `json:"connection_id"`
	GatewayID    string    `json:"gateway_id"`
	LeaseEpoch   int64     `json:"lease_epoch"`
	ExpiresAt    time.Time `json:"expires_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GatewayRegistration is the input used when a process starts.
type GatewayRegistration struct {
	GatewayID       string
	Roles           []string
	Version         string
	Address         string
	InternalAddress string
	Labels          map[string]string
}

// RuntimeRegistration is the input used when a daemon connects.
type RuntimeRegistration struct {
	DaemonID          string
	GatewayID         string
	RuntimeName       string
	RuntimeType       string
	Version           string
	MaxConcurrentRuns int
	Labels            map[string]string
	Capabilities      map[string]string
	RunningRuns       []RuntimeRunRef
}

// AppConnectionRegistration is the input used when an App connects.
type AppConnectionRegistration struct {
	ConnectionID    string
	AppID           string
	TenantID        string
	GatewayID       string
	ProtocolVersion int
	Capabilities    map[string]string
}

// Notification is one durable cross-replica wakeup.
type Notification struct {
	NotificationID int64           `json:"notification_id"`
	Channel        string          `json:"channel"`
	DedupeKey      string          `json:"dedupe_key,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	Attempts       int             `json:"attempts"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	ClaimedBy      string          `json:"claimed_by,omitempty"`
	ClaimedUntil   *time.Time      `json:"claimed_until,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
}

// CapacityLease reserves one capacity slot on a Runtime connection.
type CapacityLease struct {
	DaemonID      string    `json:"daemon_id"`
	ReservationID string    `json:"reservation_id"`
	GatewayID     string    `json:"gateway_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ReapResult reports expired leases moved offline.
type ReapResult struct {
	Gateways int64
	Runtimes int64
	Apps     int64
	Exec     int64
	Capacity int64
}
