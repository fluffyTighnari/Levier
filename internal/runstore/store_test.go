// Middleware — internal/runstore
//
// PostgreSQL RunStore integration tests.
//
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/id"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("MIDDLEWARE_TEST_POSTGRES is not set to 1")
	}
	dsn := os.Getenv("MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIDDLEWARE_TEST_DATABASE_URL is not set")
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

func createApp(t *testing.T, store *Store) App {
	t.Helper()
	app, err := store.CreateApp(context.Background(), App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	return app
}

func TestCreateRunAndGetRun(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)

	params := json.RawMessage(`{"max_steps":8}`)
	run, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "idem-1",
		RequestHash:    "hash-1",
		Runtime:        "normal",
		Strategy:       "echo",
		Params:         params,
		Model:          json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
		TraceID:        "trace-1",
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if run.Status != "queued" {
		t.Fatalf("status = %q, want queued", run.Status)
	}
	if run.RunTimeoutMS != 1_800_000 {
		t.Fatalf("timeout = %d", run.RunTimeoutMS)
	}

	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.AppID != app.AppID || got.Strategy != "echo" || got.TraceID != "trace-1" {
		t.Fatalf("run = %+v", got)
	}
	var gotParams, wantParams any
	if err := json.Unmarshal(got.Params, &gotParams); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if err := json.Unmarshal(params, &wantParams); err != nil {
		t.Fatalf("decode want params: %v", err)
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("params = %s, want %s", got.Params, params)
	}

	if _, err := store.GetRun(ctx, "missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRun(missing) error = %v, want ErrRunNotFound", err)
	}
}

func TestCreateRunIdempotency(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)

	first, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("first CreateRun() error = %v", err)
	}

	second, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("second CreateRun() error = %v", err)
	}
	if second.RunID != first.RunID {
		t.Fatalf("second run ID = %q, want %q", second.RunID, first.RunID)
	}

	_, err = store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-b",
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateRun() error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestAppendEventIsIdempotentAndAdvancesProjection(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(),
		AppID: app.AppID,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	event := Event{
		RunID:   run.RunID,
		Attempt: 1,
		Seq:     1,
		Source:  "daemon",
		Type:    "RUN_STATUS",
		Payload: json.RawMessage(`{"status":"running"}`),
	}
	result, err := store.AppendEvent(ctx, event)
	if err != nil || !result.Inserted {
		t.Fatalf("first AppendEvent() result=%+v error=%v", result, err)
	}
	result, err = store.AppendEvent(ctx, event)
	if err != nil {
		t.Fatalf("duplicate AppendEvent() error = %v", err)
	}
	if result.Inserted {
		t.Fatal("duplicate AppendEvent() inserted = true, want false")
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT count(*) FROM run_events WHERE run_id = $1", run.RunID,
	).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}

	events, err := store.ListEvents(ctx, run.RunID, 0, 0, 10)
	if err != nil || len(events) != 1 || events[0].Type != "RUN_STATUS" {
		t.Fatalf("ListEvents() = %+v, %v", events, err)
	}

	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.LastEventAttempt != 1 || got.LastEventSeq != 1 {
		t.Fatalf("projection = (%d,%d), want (1,1)", got.LastEventAttempt, got.LastEventSeq)
	}

	// A newer attempt advances the projection.
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 2, Seq: 1, Source: "daemon", Type: "RUN_STATUS",
	}); err != nil {
		t.Fatalf("append attempt 2: %v", err)
	}
	got, _ = store.GetRun(ctx, run.RunID)
	if got.LastEventAttempt != 2 || got.LastEventSeq != 1 {
		t.Fatalf("projection = (%d,%d), want (2,1)", got.LastEventAttempt, got.LastEventSeq)
	}

	// An older attempt must not move the watermark backwards.
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 9, Source: "daemon", Type: "RUN_STATUS",
	}); err != nil {
		t.Fatalf("append old attempt: %v", err)
	}
	got, _ = store.GetRun(ctx, run.RunID)
	if got.LastEventAttempt != 2 || got.LastEventSeq != 1 {
		t.Fatalf("projection moved backwards: (%d,%d)", got.LastEventAttempt, got.LastEventSeq)
	}
}

func TestAssignRunAndUpdateStatus(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(),
		AppID: app.AppID,
		Input: json.RawMessage(`[{"role":"user","content":"hello"}]`),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if string(run.Input) != `[{"role": "user", "content": "hello"}]` &&
		string(run.Input) != `[{"role":"user","content":"hello"}]` {
		t.Fatalf("input = %s", run.Input)
	}

	assigned, err := store.AssignRun(ctx, Assignment{
		RunID:              run.RunID,
		ExpectedStatus:     StatusQueued,
		ExpectedAttempt:    0,
		ExpectedLeaseEpoch: 0,
		Attempt:            1,
		LeaseEpoch:         1,
		DaemonID:           "daemon-1",
	})
	if err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}
	if assigned.Status != "assigned" || assigned.Attempt != 1 || assigned.LeaseEpoch != 1 {
		t.Fatalf("assigned = %+v", assigned)
	}
	if assigned.AssignedDaemonID != "daemon-1" || assigned.AssignedAt == nil {
		t.Fatalf("assigned daemon/time = %+v", assigned)
	}

	if _, err := store.Transition(ctx, Transition{
		RunID:              run.RunID,
		ExpectedStatus:     StatusAssigned,
		ToStatus:           StatusRunning,
		ExpectedAttempt:    1,
		ExpectedLeaseEpoch: 1,
	}); err != nil {
		t.Fatalf("Transition(running) error = %v", err)
	}
	if _, err := store.Transition(ctx, Transition{
		RunID:              run.RunID,
		ExpectedStatus:     StatusRunning,
		ToStatus:           StatusCompleted,
		ExpectedAttempt:    1,
		ExpectedLeaseEpoch: 1,
	}); err != nil {
		t.Fatalf("Transition(completed) error = %v", err)
	}
	current, _ := store.GetRun(ctx, run.RunID)
	if current.Status != "completed" {
		t.Fatalf("status = %q", current.Status)
	}

	if _, err := store.Transition(ctx, Transition{
		RunID:              run.RunID,
		ExpectedStatus:     StatusRunning,
		ToStatus:           StatusFailed,
		ExpectedAttempt:    2,
		ExpectedLeaseEpoch: 2,
	}); !errors.Is(err, ErrRunNotAssignable) {
		t.Fatalf("stale Transition() error = %v, want ErrRunNotAssignable", err)
	}
}

func TestAppendEventOutOfOrderAndGap(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})
	if _, err := store.AssignRun(ctx, Assignment{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 1, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}

	makeEvent := func(seq int64) Event {
		return Event{
			RunID: run.RunID, Attempt: 1, Seq: seq, Source: "daemon",
			Type: "RUN_STATUS", Payload: json.RawMessage(`{"status":"running"}`),
			LeaseEpoch: 1,
		}
	}
	result, err := store.AppendEvent(ctx, makeEvent(2))
	if err != nil || !result.Inserted || result.Gap {
		t.Fatalf("seq2 result = %+v, %v", result, err)
	}
	result, err = store.AppendEvent(ctx, makeEvent(1))
	if err != nil || !result.Inserted {
		t.Fatalf("seq1 result = %+v, %v", result, err)
	}
	result, err = store.AppendEvent(ctx, makeEvent(20))
	if err != nil || !result.Inserted || !result.Gap || result.GapSize != 17 {
		t.Fatalf("seq20 result = %+v, %v", result, err)
	}
	result, err = store.AppendEvent(ctx, makeEvent(2))
	if err != nil || result.Inserted {
		t.Fatalf("duplicate result = %+v, %v", result, err)
	}

	current, _ := store.GetRun(ctx, run.RunID)
	if current.LastEventAttempt != 1 || current.LastEventSeq != 20 {
		t.Fatalf("projection = (%d,%d), want (1,20)", current.LastEventAttempt, current.LastEventSeq)
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT count(*) FROM run_events WHERE run_id = $1", run.RunID,
	).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 3 {
		t.Fatalf("event count = %d, want 3", count)
	}
}

func TestAppendEventStaleAndEphemeral(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})
	if _, err := store.AssignRun(ctx, Assignment{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 2, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}

	result, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon",
		Type: "RUN_STATUS", LeaseEpoch: 1,
	})
	if err != nil || !result.Stale || result.Inserted {
		t.Fatalf("stale result = %+v, %v", result, err)
	}
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon",
		Type: "RUN_STATUS", LeaseEpoch: 2, Ephemeral: true,
	}); !errors.Is(err, ErrEphemeralEvent) {
		t.Fatalf("ephemeral error = %v", err)
	}
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 1, Source: "mw",
		Type: "RUN_STATUS", LeaseEpoch: 2,
	}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("mw seq error = %v", err)
	}
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: MWSeqBase, Source: "daemon",
		Type: "RUN_STATUS", LeaseEpoch: 2,
	}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("daemon reserved seq error = %v", err)
	}
}

func TestListEventsPaginationNoDuplicatesOrMisses(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})
	for attempt := 1; attempt <= 2; attempt++ {
		maxSeq := 5
		if attempt == 2 {
			maxSeq = 3
		}
		for seq := 1; seq <= maxSeq; seq++ {
			if _, err := store.AppendEvent(ctx, Event{
				RunID: run.RunID, Attempt: attempt, Seq: int64(seq), Source: "daemon",
				Type: "MESSAGE_COMPLETED", Payload: json.RawMessage(`{"ok":true}`),
			}); err != nil {
				t.Fatalf("append (%d,%d): %v", attempt, seq, err)
			}
		}
	}

	var afterAttempt int32
	var afterSeq int64
	var collected []Event
	for {
		page, err := store.ListEvents(ctx, run.RunID, afterAttempt, afterSeq, 2)
		if err != nil {
			t.Fatalf("ListEvents() error = %v", err)
		}
		if len(page) == 0 {
			break
		}
		collected = append(collected, page...)
		last := page[len(page)-1]
		afterAttempt = int32(last.Attempt)
		afterSeq = last.Seq
	}
	if len(collected) != 8 {
		t.Fatalf("collected %d events, want 8", len(collected))
	}
	seen := make(map[string]bool)
	wantAttempt, wantSeq := 1, int64(1)
	for _, event := range collected {
		key := fmt.Sprintf("%d/%d", event.Attempt, event.Seq)
		if seen[key] {
			t.Fatalf("duplicate event %s", key)
		}
		seen[key] = true
		if event.Attempt != wantAttempt || event.Seq != wantSeq {
			t.Fatalf("event = (%d,%d), want (%d,%d)", event.Attempt, event.Seq, wantAttempt, wantSeq)
		}
		wantSeq++
		if wantSeq > 5 && wantAttempt == 1 {
			wantAttempt = 2
			wantSeq = 1
		}
	}

	capped, err := store.ListEvents(ctx, run.RunID, 0, 0, 5000)
	if err != nil || len(capped) != 8 {
		t.Fatalf("capped ListEvents() = %d, %v; want 8", len(capped), err)
	}
}

func TestListSessionEventsAcrossRuns(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	first, _ := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(), AppID: app.AppID,
		Runtime: "longterm", SessionID: "session-1",
	})
	time.Sleep(2 * time.Millisecond)
	second, _ := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(), AppID: app.AppID,
		Runtime: "longterm", SessionID: "session-1",
	})
	for _, run := range []Run{first, second} {
		for seq := int64(1); seq <= 2; seq++ {
			if _, err := store.AppendEvent(ctx, Event{
				RunID: run.RunID, Attempt: 1, Seq: seq, Source: "daemon",
				Type: "MESSAGE_COMPLETED", Payload: json.RawMessage(`{"ok":true}`),
			}); err != nil {
				t.Fatalf("append session event: %v", err)
			}
		}
	}

	events, err := store.ListSessionEvents(ctx, "session-1", 0, 0, 10)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("session events = %d, want 4", len(events))
	}
	if events[0].RunID != first.RunID || events[2].RunID != second.RunID {
		t.Fatalf("session order = %+v", events)
	}
	if events[0].Seq != 1 || events[1].Seq != 2 || events[2].Seq != 1 || events[3].Seq != 2 {
		t.Fatalf("session seq order = %+v", events)
	}

	page, err := store.ListSessionEvents(ctx, "session-1", 1, 1, 10)
	if err != nil || len(page) != 2 {
		t.Fatalf("session page = %+v, %v; want 2", page, err)
	}
}

func TestConcurrentTransitionOnlyOneSucceeds(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})
	if _, err := store.AssignRun(ctx, Assignment{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 1, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Transition(ctx, Transition{
				RunID: run.RunID, ExpectedStatus: StatusAssigned,
				ToStatus: StatusRunning, ExpectedAttempt: 1, ExpectedLeaseEpoch: 1,
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrRunNotAssignable) {
			failures++
		} else {
			t.Fatalf("Transition() error = %v", err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("successes=%d failures=%d", successes, failures)
	}
}

func TestIllegalTransitionAndTerminalGuard(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})

	if _, err := store.Transition(ctx, Transition{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ToStatus: StatusRunning, ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
	}); !errors.Is(err, ErrRunNotAssignable) && !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("illegal queued->running error = %v", err)
	}

	if _, err := store.AssignRun(ctx, Assignment{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 1, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}
	if _, err := store.Transition(ctx, Transition{
		RunID: run.RunID, ExpectedStatus: StatusAssigned,
		ToStatus: StatusRunning, ExpectedAttempt: 1, ExpectedLeaseEpoch: 1,
	}); err != nil {
		t.Fatalf("assigned->running error = %v", err)
	}
	if _, err := store.Transition(ctx, Transition{
		RunID: run.RunID, ExpectedStatus: StatusRunning,
		ToStatus: StatusCompleted, ExpectedAttempt: 1, ExpectedLeaseEpoch: 1,
	}); err != nil {
		t.Fatalf("running->completed error = %v", err)
	}
	if _, err := store.Transition(ctx, Transition{
		RunID: run.RunID, ExpectedStatus: StatusCompleted,
		ToStatus: StatusRunning, ExpectedAttempt: 1, ExpectedLeaseEpoch: 1,
	}); !errors.Is(err, ErrRunNotAssignable) {
		t.Fatalf("terminal transition error = %v", err)
	}
}

func TestEventDoesNotChangeRunStatus(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})

	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon",
		Type: "RUN_STATUS", Payload: json.RawMessage(`{"status":"running"}`),
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	current, _ := store.GetRun(ctx, run.RunID)
	if current.Status != StatusQueued {
		t.Fatalf("status = %q, want queued; events must not write status", current.Status)
	}
}

func TestListActiveRunsAndRequeue(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})
	if _, err := store.AssignRun(ctx, Assignment{
		RunID: run.RunID, ExpectedStatus: StatusQueued,
		ExpectedAttempt: 0, ExpectedLeaseEpoch: 0,
		Attempt: 1, LeaseEpoch: 1, DaemonID: "daemon-1",
	}); err != nil {
		t.Fatalf("AssignRun() error = %v", err)
	}
	active, err := store.ListActiveRunsForDaemon(ctx, "daemon-1")
	if err != nil || len(active) != 1 || active[0].RunID != run.RunID {
		t.Fatalf("active = %+v, %v", active, err)
	}
	requeued, err := store.RequeueRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("RequeueRun() error = %v", err)
	}
	if requeued.Status != StatusQueued || requeued.Attempt != 1 || requeued.LeaseEpoch != 2 {
		t.Fatalf("requeued = %+v", requeued)
	}
	if requeued.AssignedDaemonID != "" || requeued.AssignedAt != nil {
		t.Fatalf("assignment was not cleared: %+v", requeued)
	}
	active, err = store.ListActiveRunsForDaemon(ctx, "daemon-1")
	if err != nil || len(active) != 0 {
		t.Fatalf("active after requeue = %+v, %v", active, err)
	}
}

func TestRunPersistsAcrossStoreInstances(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(),
		AppID: app.AppID,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	other := New(pool)
	got, err := other.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("second Store GetRun() error = %v", err)
	}
	if got.RunID != run.RunID {
		t.Fatalf("run ID = %q, want %q", got.RunID, run.RunID)
	}
}

func TestInvalidEventRejected(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})

	for _, event := range []Event{
		{RunID: run.RunID, Attempt: 0, Seq: 1, Source: "daemon", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 0, Source: "daemon", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "bad", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon", Type: ""},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon", Type: "RUN_STATUS", Payload: json.RawMessage(`{bad`)},
	} {
		if _, err := store.AppendEvent(ctx, event); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("AppendEvent(%+v) error = %v, want ErrInvalidEvent", event, err)
		}
	}
}
