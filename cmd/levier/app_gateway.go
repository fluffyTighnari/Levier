// Levier - app-gateway role assembly.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/app/appgateway"
	"github.com/fluffyTighnari/levier/internal/state/cluster"
	"github.com/fluffyTighnari/levier/internal/platform/config"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/app/httpapi"
	"github.com/fluffyTighnari/levier/internal/runtime/internalapi"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
)

// runAppGatewayOnly serves the App WebSocket role on its own listener.
func runAppGatewayOnly(
	ctx context.Context,
	cfg config.Config,
	pool *pgxpool.Pool,
	internalServer *internalapi.Server,
	logger *slog.Logger,
) {
	if internalServer == nil {
		logger.Error("app-gateway role requires grpc.internal_addr and grpc.internal_token")
		return
	}
	baseHub := connection.NewAppHub()
	defer baseHub.Close()
	store := cluster.New(pool)
	startConnectionReaper(ctx, store, cfg.Workflow.ReapInterval.Std(), cfg.Workflow.ReapLimit, logger)
	appHub := appgateway.NewTrackedHub(
		baseHub,
		store,
		cfg.Gateway.InstanceID,
		cfg.Gateway.LeaseTTL.Std(),
		logger,
	)
	go appHub.RunHeartbeat(ctx)

	appgateway.New(baseHub).WithLeaseStore(store, cfg.Gateway.InstanceID).
		Register(internalServer.GRPC())
	internalServer.Serve()

	runs := runstore.New(pool)
	api := httpapi.NewServer(runs, nil, httpapi.Config{
		WSTokenTTL:    cfg.HTTP.WSTokenTTL.Std(),
		WSTokenSecret: cfg.HTTP.WSTokenSecret,
		PublicWSURL:   cfg.HTTP.PublicWSURL,
		AppHub:        appHub,
		Logger:        logger,
	})

	mux := http.NewServeMux()
	mux.Handle(httpapi.APIPrefix+"/ws", api.WSHandler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.HTTP.ReadTimeout.Std(),
		WriteTimeout: cfg.HTTP.WriteTimeout.Std(),
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("app gateway http server stopped", "err", err)
		}
	}()
	logger.Info("levier app-gateway ready",
		"http_addr", cfg.HTTP.Addr,
		"internal_grpc_addr", cfg.GRPC.InternalAddr,
	)
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout.Std())
	_ = server.Shutdown(shutdownCtx)
	cancel()
}
