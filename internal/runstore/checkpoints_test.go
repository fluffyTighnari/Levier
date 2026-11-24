// Levier - session checkpoint persistence tests.
// Copyright 2026 The Levier Authors.

package runstore

import (
	"context"
	"errors"
	"testing"

	"github.com/fluffyTighnari/levier/internal/id"
)

func TestSaveAndGetCheckpoint(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID:       "run-" + id.New(),
		AppID:       app.AppID,
		TenantID:    app.TenantID,
		Runtime:     "longterm",
		RuntimeName: "graph1",
		SessionID:   "session-" + id.New(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	state := []byte(`{"step":12,"node":"approval"}`)
	if err := store.SaveCheckpoint(ctx, Checkpoint{
		SessionID:      run.SessionID,
		AppID:          run.AppID,
		TenantID:       run.TenantID,
		RunID:          run.RunID,
		State:          state,
		EventAttempt:   1,
		EventWatermark: 42,
	}); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}

	checkpoint, err := store.GetCheckpoint(ctx, run.SessionID, run.AppID, run.TenantID)
	if err != nil {
		t.Fatalf("GetCheckpoint() error = %v", err)
	}
	if string(checkpoint.State) != string(state) {
		t.Fatalf("state = %s", checkpoint.State)
	}
	if checkpoint.EventAttempt != 1 || checkpoint.EventWatermark != 42 || checkpoint.RunID != run.RunID {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}

	if _, err := store.GetCheckpoint(ctx, run.SessionID, "other-app", run.TenantID); !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("cross-app checkpoint error = %v", err)
	}
}

func TestSaveCheckpointRejectsSessionOwnerMismatch(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	first := createApp(t, store)
	second := createApp(t, store)
	sessionID := "session-" + id.New()

	if err := store.SaveCheckpoint(ctx, Checkpoint{
		SessionID: sessionID,
		AppID:     first.AppID,
		TenantID:  first.TenantID,
		State:     []byte(`{"step":1}`),
	}); err != nil {
		t.Fatalf("first SaveCheckpoint() error = %v", err)
	}
	if err := store.SaveCheckpoint(ctx, Checkpoint{
		SessionID: sessionID,
		AppID:     second.AppID,
		TenantID:  second.TenantID,
		State:     []byte(`{"step":2}`),
	}); !errors.Is(err, ErrCheckpointConflict) {
		t.Fatalf("conflicting SaveCheckpoint() error = %v", err)
	}
}
