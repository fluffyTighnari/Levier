// Levier - Runtime Gateway database lifecycle.
//
// Copyright 2026 The Levier Authors.
package runtimegateway

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
)

// Registry mirrors daemon connections into PostgreSQL leases.
type Registry struct {
	store     *cluster.Store
	gatewayID string
	ttl       time.Duration
	hub       *connection.Hub
	logger    *slog.Logger
	sink      DaemonLifecycleSink

	mu       sync.Mutex
	epochs   map[string]int64
	arrivals map[string]time.Time
}

// DaemonLifecycleSink forwards daemon lifecycle snapshots to Control.
type DaemonLifecycleSink interface {
	DaemonOnline(info connection.DaemonInfo) error
	DaemonOffline(info connection.DaemonInfo) error
}

// SetSink installs a Control lifecycle forwarder.
func (r *Registry) SetSink(sink DaemonLifecycleSink) {
	r.sink = sink
}

// NewRegistry returns a Runtime connection lease registry.
func NewRegistry(store *cluster.Store, gatewayID string, ttl time.Duration, hub *connection.Hub, logger *slog.Logger) *Registry {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		store:     store,
		gatewayID: gatewayID,
		ttl:       ttl,
		hub:       hub,
		logger:    logger,
		epochs:    make(map[string]int64),
		arrivals:  make(map[string]time.Time),
	}
}

// OnHello registers a live daemon connection.
func (r *Registry) OnHello(info connection.DaemonInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	running := make([]cluster.RuntimeRunRef, 0, len(info.RunningRuns))
	for _, run := range info.RunningRuns {
		running = append(running, cluster.RuntimeRunRef{RunID: run.RunID, LeaseEpoch: run.LeaseEpoch})
	}
	registered, err := r.store.RegisterRuntime(ctx, cluster.RuntimeRegistration{
		DaemonID:          info.DaemonID,
		GatewayID:         r.gatewayID,
		RuntimeName:       info.RuntimeName,
		RuntimeType:       runtimeTypeName(info.RuntimeType.String()),
		Version:           info.Version,
		MaxConcurrentRuns: int(info.MaxConcurrentRuns),
		Labels:            info.Labels,
		Capabilities:      info.Capabilities,
		RunningRuns:       running,
	}, r.ttl)
	if err != nil {
		r.logger.Error("register runtime failed; disconnecting daemon",
			"daemon_id", info.DaemonID,
			"gateway_id", r.gatewayID,
			"err", err,
		)
		r.hub.Disconnect(info.DaemonID)
		return
	}
	r.mu.Lock()
	r.epochs[info.DaemonID] = registered.ConnectionEpoch
	r.arrivals[info.DaemonID] = info.ConnectedAt
	r.mu.Unlock()
	r.logger.Info("runtime connection registered",
		"daemon_id", info.DaemonID,
		"gateway_id", r.gatewayID,
		"connection_epoch", registered.ConnectionEpoch,
	)
	if r.sink != nil {
		if err := r.sink.DaemonOnline(info); err != nil {
			r.logger.Warn("forward daemon online failed",
				"daemon_id", info.DaemonID,
				"err", err,
			)
		}
	}
}

// OnHeartbeat renews the connection lease and capacity snapshot.
func (r *Registry) OnHeartbeat(daemonID string, runningCount int32) {
	r.mu.Lock()
	epoch, ok := r.epochs[daemonID]
	r.mu.Unlock()
	if !ok {
		return
	}
	info, ok := r.hub.Get(daemonID)
	if !ok {
		return
	}
	running := make([]cluster.RuntimeRunRef, 0, len(info.RunningRuns))
	for _, run := range info.RunningRuns {
		running = append(running, cluster.RuntimeRunRef{RunID: run.RunID, LeaseEpoch: run.LeaseEpoch})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := r.store.HeartbeatRuntime(
		ctx,
		daemonID,
		r.gatewayID,
		epoch,
		int(runningCount),
		int(info.Reserved),
		running,
		r.ttl,
	)
	if errors.Is(err, cluster.ErrLeaseLost) {
		r.logger.Warn("runtime lease lost; disconnecting daemon",
			"daemon_id", daemonID,
			"gateway_id", r.gatewayID,
			"connection_epoch", epoch,
		)
		r.hub.Disconnect(daemonID)
		return
	}
	if err != nil {
		r.logger.Warn("heartbeat runtime failed",
			"daemon_id", daemonID,
			"gateway_id", r.gatewayID,
			"err", err,
		)
	}
}

// OnOffline marks the Runtime connection offline only when the callback still
// belongs to the most recently registered connection generation.
func (r *Registry) OnOfflineInfo(info connection.DaemonInfo) {
	r.mu.Lock()
	epoch, hasEpoch := r.epochs[info.DaemonID]
	arrival, _ := r.arrivals[info.DaemonID]
	stale := !hasEpoch || (!info.ConnectedAt.IsZero() && info.ConnectedAt != arrival)
	if stale {
		r.mu.Unlock()
		r.logger.Debug("ignore stale runtime offline callback",
			"daemon_id", info.DaemonID,
			"connection_epoch", epoch,
		)
		return
	}
	delete(r.epochs, info.DaemonID)
	delete(r.arrivals, info.DaemonID)
	r.mu.Unlock()
	if !hasEpoch {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.store.MarkRuntimeOffline(ctx, info.DaemonID, epoch)
	if errors.Is(err, cluster.ErrLeaseLost) {
		return
	}
	if err != nil {
		r.logger.Warn("mark runtime offline failed",
			"daemon_id", info.DaemonID,
			"gateway_id", r.gatewayID,
			"connection_epoch", epoch,
			"err", err,
		)
	}
	if r.sink != nil {
		if err := r.sink.DaemonOffline(info); err != nil {
			r.logger.Warn("forward daemon offline failed",
				"daemon_id", info.DaemonID,
				"err", err,
			)
		}
	}
}

func runtimeTypeName(value string) string {
	switch strings.ToLower(value) {
	case "sandbox":
		return "sandbox"
	case "longterm":
		return "longterm"
	default:
		return "normal"
	}
}
