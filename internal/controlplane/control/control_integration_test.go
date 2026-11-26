// Levier — internal/controlplane/control
//
// Control + DaemonHub + RunStore + workflow integration test.
//
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/platform/db"
	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/testing/testutil"
	"github.com/fluffyTighnari/levier/internal/testing/testutil/fakedaemon"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testutil.EnvOrLegacy("LEVIER_TEST_POSTGRES", "MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("LEVIER_TEST_POSTGRES is not set to 1")
	}
	dsn := testutil.EnvOrLegacy("LEVIER_TEST_DATABASE_URL", "MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEVIER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := strings.ToLower("mw_test_" + id.New())
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

func TestDispatchEchoRunEndToEnd(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	run, err := runs.CreateRun(ctx, runstore.Run{
		RunID:       "run-" + id.New(),
		AppID:       app.AppID,
		Runtime:     "normal",
		RuntimeName: "echo1",
		Input:       json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Model:       json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	hub := connection.New(connection.Config{
		Authenticate: func(daemonID, token string) bool {
			return daemonID == "daemon-1" && token == "token-1"
		},
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	hub.Register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		hub.Close()
		_ = listener.Close()
	})

	ctrl := New(runs, hub, Config{AckTimeout: 2 * time.Second})
	hub.SetOnEnvelope(ctrl.HandleEnvelope)
	ctrl.Start(ctx)
	t.Cleanup(ctrl.Close)

	queue := workflow.NewPostgres(pool)
	workers := workflow.NewPool(queue, ctrl, 1, workflow.WorkerConfig{
		Owner:             "control-test",
		LeaseTTL:          5 * time.Second,
		PollInterval:      10 * time.Millisecond,
		HeartbeatInterval: 100 * time.Millisecond,
		ShutdownTimeout:   time.Second,
	})
	workers.Start()
	t.Cleanup(workers.Stop)

	daemonCfg := fakedaemon.DefaultConfig()
	daemonCfg.DaemonID = "daemon-1"
	daemonCfg.AuthToken = "token-1"
	daemonCfg.RuntimeName = "echo1"
	daemonCfg.HeartbeatInterval = 20 * time.Millisecond
	daemonCfg.AutoEcho = true
	daemon, err := fakedaemon.Dial(ctx, listener.Addr().String(), daemonCfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = daemon.Close() })

	if _, err := queue.Enqueue(ctx, workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := runs.GetRun(ctx, run.RunID)
		if err == nil && current.Status == "completed" {
			if current.Attempt != 1 || current.LeaseEpoch != 1 {
				t.Fatalf("attempt/epoch = %d/%d", current.Attempt, current.LeaseEpoch)
			}
			if current.AssignedDaemonID != "daemon-1" {
				t.Fatalf("assigned daemon = %q", current.AssignedDaemonID)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_events WHERE run_id = $1", run.RunID).Scan(&count); err != nil {
				t.Fatalf("count events: %v", err)
			}
			if count != 3 {
				t.Fatalf("event count = %d, want 3", count)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	current, _ := runs.GetRun(ctx, run.RunID)
	t.Fatalf("run status = %q, want completed", current.Status)
}

func TestWaitingResumeUsesPersistedCheckpoint(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	sessionID := "session-" + id.New()
	if _, err := runs.EnsureSession(ctx, sessionID, app.AppID, app.TenantID); err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	run, err := runs.CreateRun(ctx, runstore.Run{
		RunID:       "run-" + id.New(),
		AppID:       app.AppID,
		Runtime:     "longterm",
		RuntimeName: "longterm1",
		SessionID:   sessionID,
		Input:       json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Model:       json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := runs.AssignRun(ctx, runstore.Assignment{
		RunID: run.RunID, ExpectedStatus: runstore.StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 1, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}
	if _, err := runs.Transition(ctx, runstore.Transition{
		RunID: run.RunID, ExpectedStatus: runstore.StatusAssigned,
		ToStatus: runstore.StatusRunning, ExpectedAttempt: 1, ExpectedLeaseEpoch: 1,
	}); err != nil {
		t.Fatalf("Transition(running) error = %v", err)
	}
	if err := runs.SaveCheckpoint(ctx, runstore.Checkpoint{
		SessionID: sessionID,
		AppID:     app.AppID,
		TenantID:  app.TenantID,
		RunID:     run.RunID,
		State:     []byte(`{"step":7}`),
	}); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}

	hub := &fakeHub{daemonID: "daemon-1"}
	tasks := &fakeTaskCanceller{}
	ctrl := New(runs, hub, Config{Checkpoints: runs, Tasks: tasks})
	ctrl.Start(ctx)
	t.Cleanup(ctrl.Close)

	data, _ := structpb.NewStruct(map[string]any{"status": "waiting"})
	waiting, err := protocol.NewRun(run.RunID, 1, 1, &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon, Data: data,
	})
	if err != nil {
		t.Fatalf("NewRun(waiting) error = %v", err)
	}
	ctrl.HandleEnvelope("daemon-1", waiting)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := runs.GetRun(ctx, run.RunID)
		if getErr == nil && current.Status == runstore.StatusWaiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := runs.GetRun(ctx, run.RunID)
	if err != nil || current.Status != runstore.StatusWaiting {
		t.Fatalf("run status = %+v, %v; want waiting", current, err)
	}

	resumed, err := ctrl.ResumeRun(ctx, run.RunID, json.RawMessage(`"approved"`))
	if err != nil {
		t.Fatalf("ResumeRun() error = %v", err)
	}
	if resumed.Status != runstore.StatusQueued || resumed.LeaseEpoch != 2 {
		t.Fatalf("resumed = %+v", resumed)
	}
	if tasks.enqueueCalls != 2 {
		t.Fatalf("enqueue calls = %d, want 2", tasks.enqueueCalls)
	}

	hub.onSend = func(env *runtimev1.Envelope) {
		go ctrl.HandleEnvelope("daemon-1", &runtimev1.Envelope{
			CorrelationId: env.GetMsgId(),
			Payload: &runtimev1.Envelope_TaskAck{
				TaskAck: &runtimev1.TaskAck{Accepted: true},
			},
		})
	}
	if err := ctrl.Handle(ctx, workflow.Task{RunID: run.RunID, Type: workflow.TaskDispatch}, workflow.Lease{}); err != nil {
		t.Fatalf("Handle(resumed) error = %v", err)
	}
	if len(hub.sent) != 1 {
		t.Fatalf("sent = %d", len(hub.sent))
	}
	assign := hub.sent[0].GetTaskAssign()
	if assign == nil || hub.sent[0].GetAttempt() != 2 {
		t.Fatalf("assign = %+v attempt=%d", assign, hub.sent[0].GetAttempt())
	}
	resume := assign.GetResumeFrom()
	if resume == nil || string(resume.GetState()) != `{"step":7}` {
		t.Fatalf("resume = %+v", resume)
	}
	if string(assign.GetResumeValueJson()) != `"approved"` {
		t.Fatalf("resume value = %s", assign.GetResumeValueJson())
	}
}

func TestCancelQueuedRunWritesMWEvent(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	run, err := runs.CreateRun(ctx, runstore.Run{
		RunID:       "run-" + id.New(),
		AppID:       app.AppID,
		Runtime:     "normal",
		RuntimeName: "echo1",
		Input:       json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Model:       json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	ctrl := New(runs, &fakeHub{}, Config{})
	cancelled, err := ctrl.CancelRun(ctx, run.RunID, "user")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if cancelled.Status != runstore.StatusCancelled {
		t.Fatalf("status = %q", cancelled.Status)
	}

	events, err := runs.ListEvents(ctx, run.RunID, 0, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	event := events[0]
	if event.Source != "mw" || event.Type != "RUN_STATUS" || event.Seq < runstore.MWSeqBase {
		t.Fatalf("event = %+v", event)
	}
	if event.Attempt != 0 {
		t.Fatalf("attempt = %d, want 0", event.Attempt)
	}
	var data map[string]string
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if data["status"] != "cancelled" || data["reason"] != "user" {
		t.Fatalf("payload = %s", event.Payload)
	}
}
