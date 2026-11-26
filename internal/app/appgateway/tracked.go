// Levier - tracked App connection lifecycle.
//
// Copyright 2026 The Levier Authors.
package appgateway

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/state/cluster"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
)

// TrackedHub wraps AppHub with PostgreSQL connection and execution leases.
type TrackedHub struct {
	*connection.AppHub
	store     *cluster.Store
	gatewayID string
	ttl       time.Duration
	logger    *slog.Logger

	mu          sync.Mutex
	epochs      map[string]int64
	leaseEpochs map[string]int64
}

// NewTrackedHub returns an AppHub that mirrors connections into PostgreSQL.
func NewTrackedHub(
	hub *connection.AppHub,
	store *cluster.Store,
	gatewayID string,
	ttl time.Duration,
	logger *slog.Logger,
) *TrackedHub {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TrackedHub{
		AppHub:      hub,
		store:       store,
		gatewayID:   gatewayID,
		ttl:         ttl,
		logger:      logger,
		epochs:      make(map[string]int64),
		leaseEpochs: make(map[string]int64),
	}
}

// Register adds a tracked App connection with a generated connection ID.
func (h *TrackedHub) Register(appID string, sink connection.EventSink) *connection.AppConn {
	return h.RegisterWithID(appID, sink, id.New())
}

// RegisterWithID registers one App connection and acquires its execution lease
// when possible.
func (h *TrackedHub) RegisterWithID(appID string, sink connection.EventSink, connectionID string) *connection.AppConn {
	conn := h.AppHub.RegisterWithID(appID, sink, connectionID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	registered, err := h.store.RegisterAppConnection(ctx, cluster.AppConnectionRegistration{
		ConnectionID:    connectionID,
		AppID:           appID,
		TenantID:        runstore.DefaultTenantID,
		GatewayID:       h.gatewayID,
		ProtocolVersion: 1,
	}, h.ttl)
	if err != nil {
		h.logger.Warn("register app connection failed",
			"app_id", appID,
			"connection_id", connectionID,
			"err", err,
		)
		h.AppHub.CloseConn(conn)
		return conn
	}
	h.mu.Lock()
	h.epochs[connectionID] = registered.ConnectionEpoch
	h.mu.Unlock()
	if lease, err := h.store.AcquireAppExecutionLease(ctx, appID, connectionID, h.gatewayID, h.ttl); err == nil {
		h.mu.Lock()
		h.leaseEpochs[connectionID] = lease.LeaseEpoch
		h.mu.Unlock()
	} else if err != cluster.ErrLeaseHeld {
		h.logger.Warn("acquire app execution lease failed",
			"app_id", appID,
			"connection_id", connectionID,
			"err", err,
		)
	}
	return conn
}

// UpdateConnectionMetadata mirrors the negotiated protocol and capabilities
// after the App Bridge hello. Registration happens before hello so a rejected
// token never creates a lease; metadata is therefore a follow-up update.
func (h *TrackedHub) UpdateConnectionMetadata(
	appID, connectionID string,
	protocolVersion int,
	capabilities []string,
) {
	values := make(map[string]string, len(capabilities))
	for _, capability := range capabilities {
		if capability != "" {
			values[capability] = "true"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.store.UpdateAppConnectionMetadata(ctx, connectionID, protocolVersion, values); err != nil &&
		err != cluster.ErrLeaseLost {
		h.logger.Warn("update app connection metadata failed",
			"app_id", appID,
			"connection_id", connectionID,
			"protocol_version", protocolVersion,
			"err", err,
		)
	}
}

// CloseConn removes one tracked App connection.
func (h *TrackedHub) CloseConn(conn *connection.AppConn) {
	if conn == nil {
		return
	}
	h.mu.Lock()
	epoch := h.epochs[conn.ConnectionID()]
	leaseEpoch := h.leaseEpochs[conn.ConnectionID()]
	delete(h.epochs, conn.ConnectionID())
	delete(h.leaseEpochs, conn.ConnectionID())
	h.mu.Unlock()
	h.AppHub.CloseConn(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.store.MarkAppConnectionOffline(ctx, conn.ConnectionID(), epoch); err != nil &&
		err != cluster.ErrLeaseLost {
		h.logger.Warn("mark app connection offline failed",
			"connection_id", conn.ConnectionID(),
			"err", err,
		)
	}
	if leaseEpoch > 0 {
		if _, err := h.store.ReleaseAppExecutionLease(ctx, conn.AppID(), conn.ConnectionID(), leaseEpoch); err != nil {
			h.logger.Debug("release app execution lease failed",
				"app_id", conn.AppID(),
				"connection_id", conn.ConnectionID(),
				"err", err,
			)
		}
	}
}

// RunHeartbeat renews App connection and execution leases until ctx is done.
func (h *TrackedHub) RunHeartbeat(ctx context.Context) {
	interval := h.ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		h.mu.Lock()
		epochs := make(map[string]int64, len(h.epochs))
		for connectionID, epoch := range h.epochs {
			epochs[connectionID] = epoch
		}
		h.mu.Unlock()
		for connectionID, epoch := range epochs {
			heartbeatCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			updated, err := h.store.HeartbeatAppConnection(heartbeatCtx, connectionID, h.gatewayID, epoch, h.ttl)
			if err == cluster.ErrLeaseLost {
				h.mu.Lock()
				delete(h.epochs, connectionID)
				h.mu.Unlock()
			} else if err == nil {
				h.mu.Lock()
				h.epochs[connectionID] = updated.ConnectionEpoch
				h.mu.Unlock()
				lease, leaseErr := h.store.AcquireAppExecutionLease(heartbeatCtx, updated.AppID, connectionID, h.gatewayID, h.ttl)
				if leaseErr == nil {
					h.mu.Lock()
					h.leaseEpochs[connectionID] = lease.LeaseEpoch
					h.mu.Unlock()
				} else if leaseErr != cluster.ErrLeaseHeld {
					h.logger.Debug("renew app execution lease failed", "connection_id", connectionID, "err", leaseErr)
				}
			}
			cancel()
			if err != nil && err != cluster.ErrLeaseLost {
				h.logger.Debug("app connection heartbeat failed", "connection_id", connectionID, "err", err)
			}
		}
	}
}
