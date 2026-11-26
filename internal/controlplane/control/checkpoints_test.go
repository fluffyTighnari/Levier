// Levier - control checkpoint handling tests.
// Copyright 2026 The Levier Authors.

package control

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
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
	registry := observe.NewRegistry()
	ctrl := New(runs, &fakeHub{}, Config{Checkpoints: checkpoints, Metrics: registry})

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
	var metrics bytes.Buffer
	if err := registry.WritePrometheus(&metrics); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	for _, want := range []string{
		`levier_checkpoints_saved_total{runtime="normal",tenant="default"} 1`,
		`levier_checkpoint_size_bytes{runtime="normal",tenant="default"} 10`,
	} {
		if !strings.Contains(metrics.String(), want) {
			t.Fatalf("metrics do not contain %q:\n%s", want, metrics.String())
		}
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
