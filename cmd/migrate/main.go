// Command migrate applies the embedded PostgreSQL migrations.
//
// Levier — cmd/migrate
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/fluffyTighnari/levier/internal/platform/config"
	"github.com/fluffyTighnari/levier/internal/platform/db"
)

func main() {
	configPath := flag.String("config", "levier.example.yaml", "path to YAML configuration")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "config", *configPath, "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, db.Config{
		URL:            cfg.Database.URL,
		MaxConns:       cfg.Database.MaxConns,
		MinConns:       cfg.Database.MinConns,
		ConnectTimeout: cfg.Database.ConnectTimeout.Std(),
		HealthTimeout:  cfg.Database.HealthTimeout.Std(),
	})
	if err != nil {
		slog.Error("connect database failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("migrate database failed", "err", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied")
}
