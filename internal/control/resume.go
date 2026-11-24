// Levier - resume a waiting longterm run.
// Copyright 2026 The Levier Authors.

package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

// ResumeRun requeues a waiting run and schedules its next dispatch attempt.
func (c *Control) ResumeRun(ctx context.Context, runID string) (runstore.Run, error) {
	c.waitingMu.Lock()
	defer c.waitingMu.Unlock()

	run, err := c.runs.GetRun(ctx, runID)
	if err != nil {
		return runstore.Run{}, err
	}
	if run.Status != runstore.StatusWaiting {
		return run, runstore.ErrRunNotResumable
	}
	requeued, err := c.runs.RequeueRun(ctx, runID)
	if err != nil {
		if errors.Is(err, runstore.ErrRunNotAssignable) {
			return runstore.Run{}, runstore.ErrRunNotResumable
		}
		return runstore.Run{}, fmt.Errorf("resume run: %w", err)
	}
	c.appendMWStatus(ctx, requeued, runstore.StatusQueued, "resume")
	if c.cfg.Tasks != nil {
		if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(
			requeued.RunID, workflow.TaskDispatch, time.Now(), 0,
		)); err != nil {
			return runstore.Run{}, fmt.Errorf("enqueue resumed run: %w", err)
		}
		timeoutAt := time.Now().Add(time.Duration(requeued.RunTimeoutMS) * time.Millisecond)
		if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(
			requeued.RunID, workflow.TaskTimeout, timeoutAt, 0,
		)); err != nil {
			return runstore.Run{}, fmt.Errorf("enqueue resumed run timeout: %w", err)
		}
	}
	c.log.Info("control run resumed",
		observe.FieldRunID, requeued.RunID,
		observe.FieldTraceID, requeued.TraceID,
		observe.FieldAppID, requeued.AppID,
		observe.FieldTenantID, requeued.TenantID,
		observe.FieldSessionID, requeued.SessionID,
	)
	return requeued, nil
}
