// Command middleware runs the middleware control plane.
//
// Middleware — cmd/middleware
//
// The current entry point loads and validates configuration, initializes
// structured logging, waits for a termination signal, and exits cleanly.
// Business packages are wired in later steps of the V2 plan.
//
// Copyright 2026 The Middleware Authors.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fluffyTighnari/middleware/internal/config"
	"github.com/fluffyTighnari/middleware/internal/observe"
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

	logger.Info("middleware starting", "config", *configPath)
	<-ctx.Done()
	logger.Info("middleware stopped")
}
