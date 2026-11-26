// Levier - internal role listener tests.
//
// Copyright 2026 The Levier Authors.
package internalapi

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestInternalListenerRequiresToken(t *testing.T) {
	server, err := Start(context.Background(), ServerOptions{
		Addr:  "127.0.0.1:0",
		Token: "internal-secret",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	good, err := DialWithTimeout(ctx, server.Addr(), "internal-secret", time.Second)
	if err != nil {
		t.Fatalf("DialWithTimeout() error = %v", err)
	}
	defer good.Close()
	if _, err := healthpb.NewHealthClient(good).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("authenticated health check error = %v", err)
	}

	bad, err := DialWithTimeout(ctx, server.Addr(), "wrong", time.Second)
	if err != nil {
		t.Fatalf("DialWithTimeout(bad) error = %v", err)
	}
	defer bad.Close()
	_, err = healthpb.NewHealthClient(bad).Check(ctx, &healthpb.HealthCheckRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated health check code = %v, want Unauthenticated", status.Code(err))
	}
}
