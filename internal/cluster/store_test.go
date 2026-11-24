// Levier - cluster registry integration tests.
// Copyright 2026 The Levier Authors.
package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/db"
	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/testutil"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testutil.EnvOrLegacy("LEVIER_TEST_POSTGRES", "MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("LEVIER_TEST_POSTGRES is not set to 1")
	}
	dsn := testutil.EnvOrLegacy("LEVIER_TEST_DATABASE_URL", "MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEVIER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := strings.ToLower("cluster_test_" + id.New())
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE")
		pool.Close()
	})
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return pool
}

func TestGatewayAndRuntimeLeases(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()

	gateway, err := store.RegisterGateway(ctx, GatewayRegistration{
		GatewayID: "gateway-1",
		Roles:     []string{"control", "runtime_gateway"},
		Version:   "test",
		Labels:    map[string]string{"zone": "a"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("RegisterGateway() error = %v", err)
	}
	if gateway.Epoch != 1 || gateway.Status != StatusOnline || gateway.Labels["zone"] != "a" {
		t.Fatalf("gateway = %+v", gateway)
	}
	heartbeat, err := store.HeartbeatGateway(ctx, gateway.GatewayID, gateway.Epoch, time.Minute)
	if err != nil || heartbeat.Epoch != gateway.Epoch {
		t.Fatalf("HeartbeatGateway() = %+v, %v", heartbeat, err)
	}
	if err := store.MarkGatewayOffline(ctx, gateway.GatewayID, gateway.Epoch); err != nil {
		t.Fatalf("MarkGatewayOffline() error = %v", err)
	}
	if _, err := store.HeartbeatGateway(ctx, gateway.GatewayID, gateway.Epoch, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("HeartbeatGateway(offline) error = %v, want ErrLeaseLost", err)
	}

	runtimeConn, err := store.RegisterRuntime(ctx, RuntimeRegistration{
		DaemonID:          "daemon-1",
		GatewayID:         "gateway-1",
		RuntimeName:       "echo1",
		RuntimeType:       "normal",
		MaxConcurrentRuns: 4,
		Capabilities:      map[string]string{"checkpoint": "true"},
		RunningRuns:       []RuntimeRunRef{{RunID: "run-1", LeaseEpoch: 2}},
	}, time.Minute)
	if err != nil {
		t.Fatalf("RegisterRuntime() error = %v", err)
	}
	if runtimeConn.ConnectionEpoch != 1 || runtimeConn.Capabilities["checkpoint"] != "true" ||
		len(runtimeConn.RunningRuns) != 1 {
		t.Fatalf("runtime connection = %+v", runtimeConn)
	}
	updated, err := store.HeartbeatRuntime(ctx, "daemon-1", "gateway-1", runtimeConn.ConnectionEpoch, 1, 0, nil, time.Minute)
	if err != nil || updated.RunningCount != 1 {
		t.Fatalf("HeartbeatRuntime() = %+v, %v", updated, err)
	}
	online, err := store.ListOnlineRuntimes(ctx)
	if err != nil || len(online) != 1 || online[0].DaemonID != "daemon-1" {
		t.Fatalf("ListOnlineRuntimes() = %+v, %v", online, err)
	}
	if err := store.MarkRuntimeOffline(ctx, "daemon-1", runtimeConn.ConnectionEpoch); err != nil {
		t.Fatalf("MarkRuntimeOffline() error = %v", err)
	}
	if _, err := store.HeartbeatRuntime(ctx, "daemon-1", "gateway-1", runtimeConn.ConnectionEpoch, 0, 0, nil, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("HeartbeatRuntime(offline) error = %v, want ErrLeaseLost", err)
	}
	reconnected, err := store.RegisterRuntime(ctx, RuntimeRegistration{
		DaemonID: "daemon-1", GatewayID: "gateway-2", RuntimeName: "echo1",
		RuntimeType: "normal", MaxConcurrentRuns: 1,
	}, time.Minute)
	if err != nil || reconnected.ConnectionEpoch != 2 || reconnected.GatewayID != "gateway-2" {
		t.Fatalf("RegisterRuntime(reconnect) = %+v, %v", reconnected, err)
	}
}

func TestAppExecutionLeaseAndNotifications(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	runs := runstore.New(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID: "app-" + id.New(), Name: "app-" + id.New(), APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	first, err := store.RegisterAppConnection(ctx, AppConnectionRegistration{
		AppID: app.AppID, TenantID: app.TenantID, GatewayID: "app-gateway-1",
		ProtocolVersion: 2, Capabilities: map[string]string{"local_tools": "true"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("RegisterAppConnection(first) error = %v", err)
	}
	second, err := store.RegisterAppConnection(ctx, AppConnectionRegistration{
		AppID: app.AppID, TenantID: app.TenantID, GatewayID: "app-gateway-2",
		ProtocolVersion: 1,
	}, time.Minute)
	if err != nil {
		t.Fatalf("RegisterAppConnection(second) error = %v", err)
	}
	if first.ConnectionEpoch != 1 || first.ProtocolVersion != 2 {
		t.Fatalf("first connection = %+v", first)
	}
	if _, err := store.HeartbeatAppConnection(ctx, first.ConnectionID, first.GatewayID, first.ConnectionEpoch, time.Minute); err != nil {
		t.Fatalf("HeartbeatAppConnection() error = %v", err)
	}
	lease, err := store.AcquireAppExecutionLease(ctx, app.AppID, first.ConnectionID, first.GatewayID, time.Minute)
	if err != nil {
		t.Fatalf("AcquireAppExecutionLease() error = %v", err)
	}
	if lease.ConnectionID != first.ConnectionID || lease.LeaseEpoch != 1 {
		t.Fatalf("lease = %+v", lease)
	}
	if _, err := store.AcquireAppExecutionLease(ctx, app.AppID, second.ConnectionID, second.GatewayID, time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("AcquireAppExecutionLease(second) error = %v, want ErrLeaseHeld", err)
	}
	released, err := store.ReleaseAppExecutionLease(ctx, app.AppID, first.ConnectionID, lease.LeaseEpoch)
	if err != nil || !released {
		t.Fatalf("ReleaseAppExecutionLease() = %v, %v", released, err)
	}
	next, err := store.AcquireAppExecutionLease(ctx, app.AppID, second.ConnectionID, second.GatewayID, time.Minute)
	if err != nil || next.ConnectionID != second.ConnectionID {
		t.Fatalf("AcquireAppExecutionLease(second after release) = %+v, %v", next, err)
	}

	notification, err := store.EnqueueNotification(ctx, "app.local_tool", json.RawMessage(fmt.Sprintf(`{"invocation_id":"%s"}`, "inv-1")))
	if err != nil {
		t.Fatalf("EnqueueNotification() error = %v", err)
	}
	pending, err := store.ListPendingNotifications(ctx, "app.local_tool", 10)
	if err != nil || len(pending) != 1 || pending[0].NotificationID != notification.NotificationID {
		t.Fatalf("ListPendingNotifications() = %+v, %v", pending, err)
	}
	if err := store.MarkNotificationPublished(ctx, notification.NotificationID); err != nil {
		t.Fatalf("MarkNotificationPublished() error = %v", err)
	}
	pending, err = store.ListPendingNotifications(ctx, "app.local_tool", 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after publish = %+v, %v", pending, err)
	}
}
