// Levier — control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/levier/internal/errs"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/protocol"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

// HandleDaemonOffline requeues runs owned by a lost daemon.
func (c *Control) HandleDaemonOffline(daemonID string) {
	ctx := context.Background()
	runs, err := c.runs.ListActiveRunsForDaemon(ctx, daemonID)
	if err != nil {
		c.log.Warn("control list daemon runs failed", "daemon_id", daemonID, "err", err)
		return
	}
	for _, run := range runs {
		c.requeueRun(ctx, run, "daemon lost")
	}
}

// RecoverQueued runs at startup to re-arm queued runs that have no active task.
func (c *Control) RecoverQueued(ctx context.Context) error {
	if c.cfg.Tasks == nil {
		return nil
	}
	runs, err := c.runs.ListActiveRuns(ctx)
	if err != nil {
		return fmt.Errorf("list active runs: %w", err)
	}
	repaired := 0
	for _, run := range runs {
		if run.Status != runstore.StatusQueued {
			continue
		}
		if err := c.ensureRunTasks(ctx, run); err != nil {
			c.log.Warn("control recover queued run failed", "run_id", run.RunID, "err", err)
			continue
		}
		repaired++
	}
	if repaired > 0 {
		c.log.Info("control recovered queued runs", "count", repaired)
	}
	return nil
}

// RecoverActive runs after the daemon reconnect grace period and requeues
// assigned/running runs whose daemon is not connected.
func (c *Control) RecoverActive(ctx context.Context) error {
	runs, err := c.runs.ListActiveRuns(ctx)
	if err != nil {
		return fmt.Errorf("list active runs: %w", err)
	}
	recovered := 0
	for _, run := range runs {
		if run.Status != runstore.StatusAssigned && run.Status != runstore.StatusRunning {
			continue
		}
		if c.cfg.DaemonAvailable != nil && c.cfg.DaemonAvailable(run.AssignedDaemonID) {
			continue
		}
		c.requeueRun(ctx, run, "startup recovery")
		recovered++
	}
	if recovered > 0 {
		c.log.Info("control recovered active runs", "count", recovered)
	}
	return nil
}

func (c *Control) ensureRunTasks(ctx context.Context, run runstore.Run) error {
	if c.cfg.Tasks == nil {
		return nil
	}
	if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
		return err
	}
	timeoutAt := run.CreatedAt
	if timeoutAt.IsZero() {
		timeoutAt = time.Now()
	}
	runTimeout := time.Duration(run.RunTimeoutMS) * time.Millisecond
	if runTimeout <= 0 {
		runTimeout = 30 * time.Minute
	}
	timeoutAt = timeoutAt.Add(runTimeout)
	if timeoutAt.Before(time.Now()) {
		timeoutAt = time.Now()
	}
	_, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(run.RunID, workflow.TaskTimeout, timeoutAt, 0))
	return err
}

// ReconcileDaemon compares Hello.running_runs with persisted run state.
func (c *Control) ReconcileDaemon(info connection.DaemonInfo) {
	ctx := context.Background()
	c.scheduleQueuedRuns(ctx, info)
	runs, err := c.runs.ListActiveRunsForDaemon(ctx, info.DaemonID)
	if err != nil {
		c.log.Warn("control reconcile list failed", "daemon_id", info.DaemonID, "err", err)
		return
	}
	running := make(map[string]connection.RunRef, len(info.RunningRuns))
	for _, run := range info.RunningRuns {
		running[run.RunID] = run
	}
	cancelIDs := make([]string, 0)
	requeue := make([]runstore.Run, 0)
	failTools := make([]runstore.Run, 0)
	for _, run := range runs {
		if runstore.TerminalStatus(run.Status) {
			if _, ok := running[run.RunID]; ok {
				cancelIDs = append(cancelIDs, run.RunID)
				failTools = append(failTools, run)
				delete(running, run.RunID)
			}
			continue
		}
		reported, ok := running[run.RunID]
		if !ok {
			if run.Status == runstore.StatusRunning || assignedBeforeHello(run, info) {
				requeue = append(requeue, run)
			}
			continue
		}
		if reported.LeaseEpoch != run.LeaseEpoch {
			cancelIDs = append(cancelIDs, run.RunID)
			requeue = append(requeue, run)
		}
		delete(running, run.RunID)
	}
	for runID := range running {
		cancelIDs = append(cancelIDs, runID)
	}
	if len(cancelIDs) > 0 {
		envelope, err := protocol.New(&runtimev1.Reconcile{CancelRunIds: cancelIDs})
		if err != nil {
			c.log.Warn("control build reconcile failed", "daemon_id", info.DaemonID, "err", err)
		} else if err := c.hub.Send(info.DaemonID, envelope); err != nil {
			c.log.Warn("control send reconcile failed", "daemon_id", info.DaemonID, "err", err)
		}
	}
	for _, run := range requeue {
		c.requeueRun(ctx, run, "daemon reconcile")
	}
	for _, run := range failTools {
		c.cancelLocalTools(run.AppID, run.RunID, string(errs.CodeRunCancelled), "run already terminal")
	}
}

// assignedBeforeHello reports whether an assigned run predates the daemon's
// current stream. Runs assigned after Hello are left alone so a concurrent
// dispatch is not mistaken for a lost run.
func assignedBeforeHello(run runstore.Run, info connection.DaemonInfo) bool {
	if run.Status != runstore.StatusAssigned {
		return false
	}
	if run.AssignedAt == nil || info.ConnectedAt.IsZero() {
		return true
	}
	return !run.AssignedAt.After(info.ConnectedAt)
}

// scheduleQueuedRuns re-arms runs left queued while no daemon was available.
// Enqueue is idempotent while a retry is pending, and creates a new task once
// the previous one is dead, so a daemon reconnect recovers the backlog.
func (c *Control) scheduleQueuedRuns(ctx context.Context, info connection.DaemonInfo) {
	if c.cfg.Tasks == nil {
		return
	}
	runs, err := c.runs.ListQueuedRuns(ctx)
	if err != nil {
		c.log.Warn("control list queued runs failed", "daemon_id", info.DaemonID, "err", err)
		return
	}
	scheduled := 0
	for _, run := range runs {
		if info.RuntimeType != runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED &&
			runtimeType(run.Runtime) != info.RuntimeType {
			continue
		}
		if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(
			run.RunID, workflow.TaskRetry, time.Now(), 0,
		)); err != nil {
			c.log.Warn("control enqueue queued run failed", "run_id", run.RunID, "err", err)
			continue
		}
		scheduled++
	}
	if scheduled > 0 {
		c.log.Info("control scheduled queued runs",
			"daemon_id", info.DaemonID,
			"count", scheduled,
		)
	}
}

func (c *Control) requeueRun(ctx context.Context, run runstore.Run, reason string) {
	requeued, err := c.runs.RequeueRun(ctx, run.RunID)
	if err != nil {
		if !errors.Is(err, runstore.ErrRunNotAssignable) {
			c.log.Warn("control requeue run failed", "run_id", run.RunID, "err", err)
		}
		return
	}
	c.appendMWStatus(ctx, requeued, runstore.StatusQueued, reason)
	c.hub.Release(run.AssignedDaemonID)
	c.cancelLocalTools(run.AppID, run.RunID, string(errs.CodeDaemonLost), reason)
	if c.cfg.Tasks != nil {
		if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(
			requeued.RunID, workflow.TaskRetry, time.Now(), 0,
		)); err != nil {
			c.log.Warn("control enqueue retry failed", "run_id", run.RunID, "err", err)
		}
	}
	c.log.Info("control run requeued",
		observe.FieldRunID, run.RunID,
		observe.FieldTraceID, run.TraceID,
		observe.FieldAppID, run.AppID,
		observe.FieldAttempt, run.Attempt,
		"lease_epoch", run.LeaseEpoch,
		"reason", reason,
	)
}

// CancelRun cancels one non-terminal run and notifies its daemon.
func (c *Control) CancelRun(ctx context.Context, runID, reason string) (runstore.Run, error) {
	run, err := c.runs.GetRun(ctx, runID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		return runstore.Run{}, err
	}
	if err != nil {
		return runstore.Run{}, fmt.Errorf("get run: %w", err)
	}
	if runstore.TerminalStatus(run.Status) {
		return run, nil
	}
	cancelled, err := c.runs.Transition(ctx, runstore.Transition{
		RunID:              run.RunID,
		ExpectedStatus:     run.Status,
		ToStatus:           runstore.StatusCancelled,
		ExpectedAttempt:    run.Attempt,
		ExpectedLeaseEpoch: run.LeaseEpoch,
	})
	if err != nil {
		if errors.Is(err, runstore.ErrRunNotAssignable) {
			current, _ := c.runs.GetRun(ctx, runID)
			return current, nil
		}
		return runstore.Run{}, fmt.Errorf("cancel run: %w", err)
	}
	c.sendCancel(ctx, cancelled, reason)
	c.appendMWStatus(ctx, cancelled, runstore.StatusCancelled, reason)
	c.hub.Release(run.AssignedDaemonID)
	c.cancelTasks(ctx, runID, reason)
	c.cancelLocalTools(run.AppID, runID, string(errs.CodeRunCancelled), reason)
	c.publishCompleted(run.AppID, runID, runstore.StatusCancelled, run.Attempt)
	c.log.Info("control run cancelled",
		observe.FieldRunID, run.RunID,
		observe.FieldTraceID, run.TraceID,
		observe.FieldAppID, run.AppID,
		observe.FieldAttempt, run.Attempt,
		"lease_epoch", run.LeaseEpoch,
		"reason", reason,
	)
	return cancelled, nil
}

func (c *Control) handleTimeout(ctx context.Context, runID string) error {
	run, err := c.runs.GetRun(ctx, runID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if runstore.TerminalStatus(run.Status) || run.Status == runstore.StatusWaiting {
		return nil
	}
	timedOut, err := c.runs.Transition(ctx, runstore.Transition{
		RunID:              run.RunID,
		ExpectedStatus:     run.Status,
		ToStatus:           runstore.StatusTimeout,
		ExpectedAttempt:    run.Attempt,
		ExpectedLeaseEpoch: run.LeaseEpoch,
	})
	if err != nil {
		if errors.Is(err, runstore.ErrRunNotAssignable) {
			return nil
		}
		return fmt.Errorf("timeout run: %w", err)
	}
	c.sendCancel(ctx, timedOut, "timeout")
	c.appendMWStatus(ctx, timedOut, runstore.StatusTimeout, "timeout")
	c.hub.Release(run.AssignedDaemonID)
	c.cancelTasks(ctx, runID, "timeout")
	c.cancelLocalTools(run.AppID, runID, string(errs.CodeRunTimeout), "run timed out")
	c.publishCompleted(run.AppID, runID, runstore.StatusTimeout, timedOut.Attempt)
	c.log.Info("control run timed out",
		observe.FieldRunID, run.RunID,
		observe.FieldTraceID, run.TraceID,
		observe.FieldAppID, run.AppID,
		observe.FieldAttempt, run.Attempt,
		"lease_epoch", run.LeaseEpoch,
	)
	return nil
}

func (c *Control) sendCancel(ctx context.Context, run runstore.Run, reason string) {
	if run.AssignedDaemonID == "" || run.Attempt < 1 {
		return
	}
	envelope, err := protocol.NewRun(run.RunID, int32(run.Attempt), run.LeaseEpoch, &runtimev1.Cancel{Reason: reason})
	if err != nil {
		c.log.Warn("control build cancel failed", "run_id", run.RunID, "err", err)
		return
	}
	if err := c.hub.Send(run.AssignedDaemonID, envelope); err != nil {
		c.log.Warn("control send cancel failed", "run_id", run.RunID, "err", err)
	}
}

func (c *Control) cancelTasks(ctx context.Context, runID, reason string) {
	if c.cfg.Tasks != nil {
		if _, err := c.cfg.Tasks.CancelRun(ctx, runID, reason); err != nil {
			c.log.Warn("control cancel tasks failed", "run_id", runID, "err", err)
		}
	}
}

func (c *Control) cancelLocalTools(appID, runID, code, message string) {
	if c.cfg.LocalTools != nil && appID != "" {
		c.cfg.LocalTools.CancelLocalTools(appID, runID, code, message)
	}
}

func (c *Control) appendMWStatus(
	ctx context.Context,
	run runstore.Run,
	status, reason string,
) {
	data := map[string]any{"status": status}
	if reason != "" {
		data["reason"] = reason
	}
	payload, err := json.Marshal(data)
	if err != nil {
		c.log.Warn("control encode mw event failed", "run_id", run.RunID, "err", err)
		return
	}
	event, err := c.runs.AppendMWEvent(ctx, runstore.Event{
		RunID:   run.RunID,
		Type:    "RUN_STATUS",
		Payload: payload,
	})
	if err != nil {
		c.log.Warn("control append mw event failed",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			"status", status,
			"reason", reason,
			"err", err,
		)
		return
	}
	if c.cfg.Publisher == nil || run.AppID == "" {
		return
	}
	message, err := json.Marshal(map[string]any{
		"type":       "event",
		"run_id":     run.RunID,
		"attempt":    event.Attempt,
		"seq":        event.Seq,
		"source":     event.Source,
		"event_type": event.Type,
		"data":       data,
		"ts_ms":      event.TSMs,
	})
	if err == nil {
		c.cfg.Publisher.Publish(run.AppID, run.RunID, message)
	}
}

func (c *Control) publishCompleted(appID, runID, status string, attempt int) {
	if c.cfg.Publisher == nil || appID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"type": "run_completed", "run_id": runID, "status": status, "attempt": attempt,
	})
	c.cfg.Publisher.Publish(appID, runID, payload)
}
