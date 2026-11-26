// Levier - runtime-gateway role assembly.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/internalapi"
	"github.com/fluffyTighnari/levier/internal/runtimegateway"
)

// runRuntimeGatewayOnly serves the daemon-facing Runtime Gateway role.
//
// Control/API runs elsewhere and receives envelopes through the internal
// ControlService. The gateway never hosts HTTP, workflow workers, or AppHub.
func runRuntimeGatewayOnly(
	ctx context.Context,
	cfg config.Config,
	pool *pgxpool.Pool,
	internalServer *internalapi.Server,
	logger *slog.Logger,
) {
	if internalServer == nil {
		logger.Error("runtime-gateway role requires grpc.internal_addr and grpc.internal_token")
		return
	}

	hub := connection.New(connection.Config{
		Authenticate: daemonAuthenticator(cfg.GRPC.DaemonTokens),
	})
	defer hub.Close()

	store := cluster.New(pool)
	startConnectionReaper(ctx, store, cfg.Workflow.ReapInterval.Std(), cfg.Workflow.ReapLimit, logger)
	runtimeLeaseTTL := cfg.Gateway.LeaseTTL.Std()
	if runtimeLeaseTTL < 5*time.Second {
		// Keep a small floor for tests and very short development leases.
		runtimeLeaseTTL = 5 * time.Second
	}
	registry := runtimegateway.NewRegistry(
		store,
		cfg.Gateway.InstanceID,
		runtimeLeaseTTL,
		hub,
		logger,
	)
	forwarder := runtimegateway.NewForwarder(
		store,
		cfg.Gateway.InstanceID,
		cfg.GRPC.InternalToken,
		3*time.Second,
		logger,
	)
	defer forwarder.Close()
	registry.SetSink(forwarder)

	hub.SetOnHello(registry.OnHello)
	hub.SetOnHeartbeat(registry.OnHeartbeat)
	hub.SetOnOfflineInfo(registry.OnOfflineInfo)
	hub.SetOnEnvelope(func(daemonID string, env *runtimev1.Envelope) {
		if err := forwarder.ForwardEnvelope(daemonID, env); err != nil {
			logger.Warn("runtime gateway forward envelope failed",
				"daemon_id", daemonID,
				"err", err,
			)
		}
	})
	runtimegateway.New(hub, 6*time.Second).
		WithOwnership(store, cfg.Gateway.InstanceID).
		Register(internalServer.GRPC())
	internalServer.Serve()

	listener, grpcServer, err := startDaemonGRPC(cfg, hub)
	if err != nil {
		logger.Error("listen grpc failed", "addr", cfg.GRPC.Addr, "err", err)
		return
	}
	defer func() {
		_ = listener.Close()
		grpcServer.Stop()
	}()

	logger.Info("levier runtime-gateway ready",
		"gateway_id", cfg.Gateway.InstanceID,
		"grpc_addr", cfg.GRPC.Addr,
		"internal_grpc_addr", cfg.GRPC.InternalAddr,
	)
	<-ctx.Done()
	logger.Info("levier runtime-gateway stopping")
	hub.Close()
	grpcServer.GracefulStop()
}
