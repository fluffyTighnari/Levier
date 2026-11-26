// Levier - session checkpoint persistence tests.
// Copyright 2026 The Levier Authors.

package runstore

import (
	"context"
	"errors"
	"testing"

	"github.com/fluffyTighnari/levier/internal/platform/id"
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

func TestEnsureSessionOwnership(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	first := createApp(t, store)
	second := createApp(t, store)
	sessionID := "session-" + id.New()

	session, err := store.EnsureSession(ctx, sessionID, first.AppID, first.TenantID)
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.SessionID != sessionID || session.AppID != first.AppID || session.Status != "active" {
		t.Fatalf("session = %+v", session)
	}
	if _, err := store.EnsureSession(ctx, sessionID, first.AppID, first.TenantID); err != nil {
		t.Fatalf("EnsureSession(existing) error = %v", err)
	}
	if _, err := store.EnsureSession(ctx, sessionID, second.AppID, second.TenantID); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("EnsureSession(other owner) error = %v", err)
	}
}

func TestListAndCloseSessions(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	other := createApp(t, store)
	sessionID := "session-" + id.New()
	if _, err := store.EnsureSession(ctx, sessionID, app.AppID, app.TenantID); err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if _, err := store.EnsureSession(ctx, "other-"+sessionID, other.AppID, other.TenantID); err != nil {
		t.Fatalf("EnsureSession(other) error = %v", err)
	}
	sessions, err := store.ListSessions(ctx, app.AppID, app.TenantID, 10)
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != sessionID {
		t.Fatalf("ListSessions() = %+v, %v", sessions, err)
	}
	closed, err := store.CloseSession(ctx, sessionID, app.AppID, app.TenantID)
	if err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	if closed.Status != "expired" || closed.ExpiresAt == nil {
		t.Fatalf("closed = %+v", closed)
	}
	if _, err := store.EnsureSession(ctx, sessionID, app.AppID, app.TenantID); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("EnsureSession(closed) error = %v, want ErrSessionClosed", err)
	}
	if _, err := store.CloseSession(ctx, sessionID, app.AppID, app.TenantID); err != nil {
		t.Fatalf("CloseSession(idempotent) error = %v", err)
	}
	if _, err := store.CloseSession(ctx, sessionID, other.AppID, other.TenantID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("CloseSession(other) error = %v, want ErrSessionNotFound", err)
	}

	activeID := "session-" + id.New()
	if _, err := store.EnsureSession(ctx, activeID, app.AppID, app.TenantID); err != nil {
		t.Fatalf("EnsureSession(active) error = %v", err)
	}
	if _, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(), AppID: app.AppID, Runtime: "longterm",
		RuntimeName: "longterm1", SessionID: activeID,
	}); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := store.CloseSession(ctx, activeID, app.AppID, app.TenantID); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("CloseSession(active) error = %v, want ErrSessionActive", err)
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
