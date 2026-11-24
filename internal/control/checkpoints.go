// Levier - checkpoint handling for longterm runs.
// Copyright 2026 The Levier Authors.

package control

import (
	"context"
	"errors"
	"fmt"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

func (c *Control) handleCheckpoint(env *runtimev1.Envelope) {
	if c == nil || c.cfg.Checkpoints == nil || env == nil {
		return
	}
	checkpoint := env.GetCheckpoint()
	if checkpoint == nil || checkpoint.GetSessionId() == "" || len(checkpoint.GetState()) == 0 {
		return
	}
	ctx := context.Background()
	if c.ctx != nil {
		ctx = c.ctx
	}
	run, err := c.runs.GetRun(ctx, env.GetRunId())
	if err != nil {
		return
	}
	if int(env.GetAttempt()) != run.Attempt || env.GetLeaseEpoch() != run.LeaseEpoch {
		return
	}
	if run.SessionID != "" && run.SessionID != checkpoint.GetSessionId() {
		c.log.Warn("control checkpoint session mismatch",
			observe.FieldRunID, run.RunID,
			observe.FieldTenantID, run.TenantID,
			"session_id", checkpoint.GetSessionId(),
			"run_session_id", run.SessionID,
		)
		return
	}
	err = c.cfg.Checkpoints.SaveCheckpoint(ctx, runstore.Checkpoint{
		SessionID:      checkpoint.GetSessionId(),
		AppID:          run.AppID,
		TenantID:       run.TenantID,
		RunID:          run.RunID,
		State:          checkpoint.GetState(),
		EventAttempt:   int(env.GetAttempt()),
		EventWatermark: checkpoint.GetEventSeqWatermark(),
	})
	if err != nil {
		if !errors.Is(err, runstore.ErrCheckpointConflict) {
			c.log.Warn("control save checkpoint failed",
				observe.FieldRunID, run.RunID,
				observe.FieldTenantID, run.TenantID,
				"err", err,
			)
		}
		return
	}
	if c.metrics != nil {
		labels := map[string]string{"tenant": run.TenantID, "runtime": run.Runtime}
		c.metrics.Inc(observe.MetricCheckpointsSavedTotal, "Checkpoints persisted by tenant and runtime.", labels)
		c.metrics.Set(observe.MetricCheckpointSizeBytes, "Last persisted checkpoint size in bytes.", labels, float64(len(checkpoint.GetState())))
	}
}

func (c *Control) resumeCheckpoint(ctx context.Context, run runstore.Run) (*runtimev1.Checkpoint, error) {
	if c == nil || c.cfg.Checkpoints == nil || run.SessionID == "" {
		return nil, nil
	}
	checkpoint, err := c.cfg.Checkpoints.GetCheckpoint(ctx, run.SessionID, run.AppID, run.TenantID)
	if errors.Is(err, runstore.ErrCheckpointNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load run checkpoint: %w", err)
	}
	if len(checkpoint.State) == 0 {
		return nil, nil
	}
	if c.metrics != nil {
		c.metrics.Inc(observe.MetricCheckpointRestoreTotal, "Checkpoint restores by tenant, runtime, and outcome.", map[string]string{
			"tenant":  run.TenantID,
			"runtime": run.Runtime,
			"result":  "restored",
		})
	}
	return &runtimev1.Checkpoint{
		SessionId:         checkpoint.SessionID,
		EventSeqWatermark: checkpoint.EventWatermark,
		State:             checkpoint.State,
	}, nil
}
