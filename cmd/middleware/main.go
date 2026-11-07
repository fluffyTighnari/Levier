// Command middleware runs the middleware control plane.
//
// Middleware — cmd/middleware
//
// Loads configuration and starts the daemon-facing gRPC RuntimeService.
// Scheduling, HTTP, and control-plane wiring land in later V2 steps.
//
// Copyright 2026 The Middleware Authors.
package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/fluffyTighnari/middleware/internal/config"
	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/control"
	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/httpapi"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

func main() {
	defaultConfig := os.Getenv("MIDDLEWARE_CONFIG")
	if defaultConfig == "" {
		defaultConfig = "config.example.yaml"
	}
	configPath := flag.String("config", defaultConfig, "path to YAML configuration")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "config", *configPath, "err", err)
		os.Exit(1)
	}

	logger, err := observe.NewLogger(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		slog.Error("initialize logger failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := db.Open(ctx, db.Config{
		URL:            cfg.Database.URL,
		MaxConns:       cfg.Database.MaxConns,
		MinConns:       cfg.Database.MinConns,
		ConnectTimeout: cfg.Database.ConnectTimeout.Std(),
		HealthTimeout:  cfg.Database.HealthTimeout.Std(),
	})
	if err != nil {
		logger.Error("connect database failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	hub := connection.New(connection.Config{
		Authenticate: daemonAuthenticator(cfg.GRPC.DaemonTokens),
	})
	defer hub.Close()

	runs := runstore.New(pool)
	controller := control.New(runs, hub, control.Config{Logger: logger})
	hub.SetOnEnvelope(controller.HandleEnvelope)
	controller.Start(ctx)
	defer controller.Close()

	queue := workflow.NewPostgres(pool)
	workers := workflow.NewPool(queue, controller, cfg.Workflow.Workers, workflow.WorkerConfig{
		Owner:        "control",
		LeaseTTL:     cfg.Workflow.LeaseTTL.Std(),
		PollInterval: cfg.Workflow.PollInterval.Std(),
	})
	workers.Start()
	defer workers.Stop()

	listener, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		logger.Error("listen grpc failed", "addr", cfg.GRPC.Addr, "err", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(cfg.GRPC.MaxRecvMsgBytes),
		grpc.MaxSendMsgSize(cfg.GRPC.MaxSendMsgBytes),
	)
	hub.Register(grpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			logger.Error("grpc server stopped", "err", err)
		}
	}()

	api := httpapi.NewServer(runs, queue, httpapi.Config{
		BearerToken:          cfg.HTTP.BearerToken,
		DefaultRunTimeoutMS:  1_800_000,
		DefaultToolTimeoutMS: 60_000,
	})
	httpListener, err := net.Listen("tcp", cfg.HTTP.Addr)
	if err != nil {
		logger.Error("listen http failed", "addr", cfg.HTTP.Addr, "err", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Handler:      api.Handler(),
		ReadTimeout:  cfg.HTTP.ReadTimeout.Std(),
		WriteTimeout: cfg.HTTP.WriteTimeout.Std(),
	}
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped", "err", err)
		}
	}()

	logger.Info("middleware starting",
		"config", *configPath,
		"database", "connected",
		"grpc_addr", cfg.GRPC.Addr,
		"http_addr", cfg.HTTP.Addr,
	)
	<-ctx.Done()

	workers.Stop()
	controller.Close()
	hub.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout.Std())
	_ = httpServer.Shutdown(shutdownCtx)
	cancel()
	_ = httpListener.Close()
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(cfg.GRPC.ShutdownTimeout.Std()):
		grpcServer.Stop()
	}
	_ = listener.Close()
	logger.Info("middleware stopped")
}

func daemonAuthenticator(tokens map[string]string) connection.Authenticator {
	return func(daemonID, token string) bool {
		expected, ok := tokens[daemonID]
		if !ok {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
	}
}
