// Levier - resume a waiting longterm run.
// Copyright 2026 The Levier Authors.

package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

// ResumeRun requeues a waiting run and schedules its next dispatch attempt.
func (c *Control) ResumeRun(ctx context.Context, runID string, resume json.RawMessage) (runstore.Run, error) {
	c.waitingMu.Lock()
	defer c.waitingMu.Unlock()

	run, err := c.runs.GetRun(ctx, runID)
	if err != nil {
		return runstore.Run{}, err
	}
	if run.Status != runstore.StatusWaiting {
		return run, runstore.ErrRunNotResumable
	}
	requeued, err := c.runs.RequeueRunWithResume(ctx, runID, resume)
	if err != nil {
		if errors.Is(err, runstore.ErrRunNotAssignable) {
			return runstore.Run{}, runstore.ErrRunNotResumable
		}
		return runstore.Run{}, fmt.Errorf("resume run: %w", err)
	}
	c.appendMWStatus(ctx, requeued, runstore.StatusQueued, "resume")
	if c.metrics != nil {
		labels := map[string]string{"tenant": requeued.TenantID, "runtime": requeued.Runtime}
		c.metrics.Inc(observe.MetricRunResumeTotal, "Waiting runs resumed by tenant and runtime.", labels)
		if !run.UpdatedAt.IsZero() {
			c.metrics.Observe(observe.MetricRunWaitingSeconds, "Time spent in the waiting state in seconds.", labels, time.Since(run.UpdatedAt).Seconds())
		}
	}
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
