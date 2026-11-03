// Command middleware runs the middleware control plane.
//
// Middleware — cmd/middleware
//
// The current entry point is a lifecycle skeleton: it starts, waits for a
// termination signal, and exits cleanly. Business packages are wired in later
// steps of the V2 plan.
//
// Copyright 2026 The Middleware Authors.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	logger.Info("middleware skeleton starting")
	<-ctx.Done()
	logger.Info("middleware skeleton stopped")
}
