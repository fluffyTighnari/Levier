// Levier - process gateway presence lease.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fluffyTighnari/levier/internal/contract/roles"
	"github.com/fluffyTighnari/levier/internal/platform/config"
	"github.com/fluffyTighnari/levier/internal/platform/version"
	"github.com/fluffyTighnari/levier/internal/state/cluster"
)

type gatewayLease struct {
	store    *cluster.Store
	gateway  cluster.GatewayInstance
	interval time.Duration
	ttl      time.Duration
	logger   *slog.Logger
	cancel   context.CancelFunc
	done     chan struct{}
}

func startGatewayLease(
	ctx context.Context,
	store *cluster.Store,
	cfg config.Config,
	roleSet roles.Set,
	logger *slog.Logger,
) (*gatewayLease, error) {
	leaseCtx, cancel := context.WithCancel(ctx)
	gateway, err := store.RegisterGateway(leaseCtx, cluster.GatewayRegistration{
		GatewayID:       cfg.Gateway.InstanceID,
		Roles:           roleSet.List(),
		Version:         version.Version,
		Address:         cfg.HTTP.Addr,
		InternalAddress: cfg.GRPC.InternalAddr,
		Labels:          map[string]string{"grpc_addr": cfg.GRPC.Addr},
	}, cfg.Gateway.LeaseTTL.Std())
	if err != nil {
		cancel()
		return nil, err
	}
	lease := &gatewayLease{
		store:    store,
		gateway:  gateway,
		interval: cfg.Gateway.HeartbeatInterval.Std(),
		ttl:      cfg.Gateway.LeaseTTL.Std(),
		logger:   logger,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go lease.heartbeat(leaseCtx)
	logger.Info("gateway registered",
		"gateway_id", gateway.GatewayID,
		"gateway_epoch", gateway.Epoch,
		"roles", gateway.Roles,
	)
	return lease, nil
}

func (g *gatewayLease) heartbeat(ctx context.Context) {
	defer close(g.done)
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gateway, err := g.store.HeartbeatGateway(ctx, g.gateway.GatewayID, g.gateway.Epoch, g.ttl)
			if err == nil {
				g.gateway = gateway
				continue
			}
			if errors.Is(err, cluster.ErrLeaseLost) {
				g.logger.Error("gateway lease lost",
					"gateway_id", g.gateway.GatewayID,
					"gateway_epoch", g.gateway.Epoch,
				)
				return
			}
			g.logger.Warn("gateway heartbeat failed",
				"gateway_id", g.gateway.GatewayID,
				"err", err,
			)
		}
	}
}

// Done closes when the heartbeat loop stops, including lease loss.
func (g *gatewayLease) Done() <-chan struct{} {
	if g == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return g.done
}

func (g *gatewayLease) Stop() {
	if g == nil {
		return
	}
	g.cancel()
	<-g.done
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.store.MarkGatewayOffline(ctx, g.gateway.GatewayID, g.gateway.Epoch); err != nil &&
		!errors.Is(err, cluster.ErrLeaseLost) {
		g.logger.Warn("mark gateway offline failed",
			"gateway_id", g.gateway.GatewayID,
			"err", err,
		)
	}
	g.logger.Info("gateway offline", "gateway_id", g.gateway.GatewayID)
}
