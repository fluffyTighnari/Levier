// Levier - control checkpoint handling tests.
// Copyright 2026 The Levier Authors.

package control

import (
	"context"
	"errors"
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/protocol"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

type fakeCheckpointStore struct {
	saved runstore.Checkpoint
	get   runstore.Checkpoint
	err   error
}

func (f *fakeCheckpointStore) SaveCheckpoint(_ context.Context, checkpoint runstore.Checkpoint) error {
	f.saved = checkpoint
	return f.err
}

func (f *fakeCheckpointStore) GetCheckpoint(_ context.Context, _, _, _ string) (runstore.Checkpoint, error) {
	if f.err != nil {
		return runstore.Checkpoint{}, f.err
	}
	if len(f.get.State) == 0 {
		return runstore.Checkpoint{}, runstore.ErrCheckpointNotFound
	}
	return f.get, nil
}

func TestHandleCheckpointPersistsState(t *testing.T) {
	run := newRun()
	run.SessionID = "session-1"
	run.Attempt = 1
	run.LeaseEpoch = 2
	runs := &fakeRuns{run: run}
	checkpoints := &fakeCheckpointStore{}
	ctrl := New(runs, &fakeHub{}, Config{Checkpoints: checkpoints})

	env, err := protocol.NewRun(run.RunID, 1, 2, &runtimev1.Checkpoint{
		SessionId:         "session-1",
		EventSeqWatermark: 7,
		State:             []byte(`{"step":3}`),
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	ctrl.HandleEnvelope("daemon-1", env)

	if checkpoints.saved.SessionID != "session-1" ||
		checkpoints.saved.RunID != run.RunID ||
		checkpoints.saved.EventWatermark != 7 ||
		string(checkpoints.saved.State) != `{"step":3}` {
		t.Fatalf("saved checkpoint = %+v", checkpoints.saved)
	}
}

func TestResumeCheckpointLoadsState(t *testing.T) {
	run := newRun()
	run.SessionID = "session-1"
	checkpoints := &fakeCheckpointStore{get: runstore.Checkpoint{
		SessionID:      "session-1",
		State:          []byte(`{"step":8}`),
		EventWatermark: 9,
	}}
	ctrl := New(&fakeRuns{run: run}, &fakeHub{}, Config{Checkpoints: checkpoints})

	resume, err := ctrl.resumeCheckpoint(context.Background(), run)
	if err != nil {
		t.Fatalf("resumeCheckpoint() error = %v", err)
	}
	if resume == nil || resume.GetSessionId() != "session-1" ||
		resume.GetEventSeqWatermark() != 9 || string(resume.GetState()) != `{"step":8}` {
		t.Fatalf("resume = %+v", resume)
	}
}

func TestResumeCheckpointMissingIsNotAnError(t *testing.T) {
	run := newRun()
	run.SessionID = "session-1"
	checkpoints := &fakeCheckpointStore{err: runstore.ErrCheckpointNotFound}
	ctrl := New(&fakeRuns{run: run}, &fakeHub{}, Config{Checkpoints: checkpoints})

	resume, err := ctrl.resumeCheckpoint(context.Background(), run)
	if err != nil || resume != nil {
		t.Fatalf("resumeCheckpoint() = %+v, %v", resume, err)
	}
	if !errors.Is(checkpoints.err, runstore.ErrCheckpointNotFound) {
		t.Fatal("unexpected fake checkpoint error")
	}
}
