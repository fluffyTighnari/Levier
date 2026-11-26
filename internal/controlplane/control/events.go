// Levier â€” control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/contract"
	"github.com/fluffyTighnari/levier/internal/platform/errs"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
)

// HandleEnvelope routes TaskAck, Event, and Checkpoint messages from DaemonHub.
func (c *Control) HandleEnvelope(daemonID string, env *runtimev1.Envelope) {
	if env == nil {
		return
	}
	if ack := env.GetTaskAck(); ack != nil {
		c.pendingMu.Lock()
		ch := c.pending[env.GetCorrelationId()]
		c.pendingMu.Unlock()
		if ch != nil {
			select {
			case ch <- ack:
			default:
			}
		}
		return
	}
	if env.GetCheckpoint() != nil {
		c.handleCheckpoint(env)
		return
	}
	if env.GetEvent() != nil {
		c.enqueueEvent(daemonID, env)
	}
}

func (c *Control) enqueueEvent(daemonID string, env *runtimev1.Envelope) {
	if c.enqueueImmediate(env) {
		return
	}
	if !criticalEvent(env.GetEvent()) {
		c.recordEventDrop("control_buffer_full", daemonID, env)
		return
	}

	timer := time.NewTimer(c.cfg.CriticalEventWait)
	defer timer.Stop()
	select {
	case c.events <- env:
		return
	case <-timer.C:
	}

	c.criticalEvents.Add(1)
	c.recordEventDrop("critical_buffer_full", daemonID, env)
	c.log.Error("control critical event buffer full",
		observe.FieldDaemonID, daemonID,
		observe.FieldRunID, env.GetRunId(),
		"event_type", runtimev1.EventType_name[int32(env.GetEvent().GetType())],
	)
	go c.dropDaemonForCriticalEvent(daemonID)
}

func (c *Control) enqueueImmediate(env *runtimev1.Envelope) bool {
	select {
	case c.events <- env:
		return true
	default:
		return false
	}
}

func (c *Control) recordEventDrop(reason, daemonID string, env *runtimev1.Envelope) {
	if c.metrics != nil {
		c.metrics.Inc(observe.MetricEventsDroppedTotal, "Dropped or skipped events by reason.", map[string]string{
			"reason": reason,
		})
	}
	c.log.Warn("control event buffer full",
		observe.FieldDaemonID, daemonID,
		observe.FieldRunID, env.GetRunId(),
		"reason", reason,
	)
}

// dropDaemonForCriticalEvent requeues the daemon's runs and closes its stream.
// The old execution is fenced by the new lease epoch.
func (c *Control) dropDaemonForCriticalEvent(daemonID string) {
	c.log.Warn("control dropping daemon after critical event loss",
		observe.FieldDaemonID, daemonID,
	)
	c.HandleDaemonOffline(daemonID)
	if c.placement != nil {
		c.placement.Disconnect(daemonID)
	}
}

func criticalEvent(event *runtimev1.Event) bool {
	if event == nil {
		return false
	}
	if event.GetType() == runtimev1.EventType_ERROR {
		return true
	}
	if event.GetType() != runtimev1.EventType_RUN_STATUS || event.GetData() == nil {
		return false
	}
	status, _ := event.GetData().AsMap()[contract.EventFieldStatus].(string)
	switch strings.ToLower(status) {
	case "completed", "failed", "cancelled", "timeout":
		return true
	}
	return false
}

func (c *Control) eventLoop() {
	defer c.wg.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case env := <-c.events:
			c.handleEvent(env)
		}
	}
}

func (c *Control) handleEvent(env *runtimev1.Envelope) {
	event := env.GetEvent()
	if event == nil {
		return
	}
	if event.GetEphemeral() {
		c.ephemeralEvents.Add(1)
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricEventsDroppedTotal, "Dropped or skipped events by reason.", map[string]string{
				"reason": "ephemeral",
			})
		}
		return
	}
	ctx := context.Background()
	run, err := c.runs.GetRun(ctx, env.GetRunId())
	if errors.Is(err, runstore.ErrRunNotFound) {
		return
	}
	if err != nil {
		c.log.Warn("control get run failed",
			observe.FieldRunID, env.GetRunId(),
			observe.FieldAttempt, env.GetAttempt(),
			"err", err,
		)
		return
	}
	if int(env.GetAttempt()) != run.Attempt || env.GetLeaseEpoch() != run.LeaseEpoch {
		return
	}

	eventData := map[string]any{}
	if event.GetData() != nil {
		eventData = event.GetData().AsMap()
	}
	payload, err := json.Marshal(eventData)
	if err != nil {
		c.log.Warn("control encode event failed", "run_id", env.GetRunId(), "err", err)
		return
	}
	source := event.GetSource()
	if source == "" {
		source = protocol.EventSourceDaemon
	}
	eventType := runtimev1.EventType_name[int32(event.GetType())]
	appendStarted := time.Now()
	result, err := c.runs.AppendEvent(ctx, runstore.Event{
		RunID:         env.GetRunId(),
		Attempt:       int(env.GetAttempt()),
		Seq:           event.GetSeq(),
		Source:        source,
		Type:          eventType,
		Name:          event.GetName(),
		SchemaVersion: event.GetSchemaVersion(),
		Payload:       payload,
		TSMs:          env.GetTsMs(),
		LeaseEpoch:    env.GetLeaseEpoch(),
	})
	if c.metrics != nil {
		c.metrics.Observe(observe.MetricEventAppendDuration, "Run event persistence latency in seconds.",
			map[string]string{"source": source, "tenant": run.TenantID}, time.Since(appendStarted).Seconds())
	}
	if err != nil {
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricEventAppendErrorsTotal, "Failed run event inserts.", map[string]string{
				"source": source, "tenant": run.TenantID,
			})
		}
		c.log.Warn("control append event failed", "run_id", env.GetRunId(), "err", err)
		return
	}
	if result.Stale {
		c.staleEvents.Add(1)
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricEventsDroppedTotal, "Dropped or skipped events by reason.", map[string]string{
				"reason": "stale_epoch",
			})
		}
		return
	}
	if result.Gap {
		c.gapEvents.Add(1)
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricEventsDroppedTotal, "Dropped or skipped events by reason.", map[string]string{
				"reason": "seq_gap",
			})
		}
		c.log.Warn("control event sequence gap",
			observe.FieldRunID, env.GetRunId(),
			observe.FieldTraceID, run.TraceID,
			observe.FieldAttempt, env.GetAttempt(),
			"seq", event.GetSeq(),
			"gap", result.GapSize,
		)
	}
	if !result.Inserted {
		return
	}
	if c.metrics != nil {
		c.metrics.Inc(observe.MetricEventsTotal, "Persisted events by type.", map[string]string{
			"type":   eventType,
			"tenant": run.TenantID,
		})
		if event.GetType() == runtimev1.EventType_CUSTOM {
			c.metrics.Inc(observe.MetricCustomEventsTotal, "Persisted custom events by name.", map[string]string{
				"name":   firstNonEmpty(event.GetName(), "unnamed"),
				"tenant": run.TenantID,
			})
		}
		c.observeUsage(run, event, eventData)
	}
	if c.cfg.Publisher != nil {
		wsPayload, err := json.Marshal(map[string]any{
			"type":           "event",
			"run_id":         env.GetRunId(),
			"attempt":        env.GetAttempt(),
			"seq":            event.GetSeq(),
			"source":         source,
			"event_type":     eventType,
			"event_name":     event.GetName(),
			"schema_version": event.GetSchemaVersion(),
			"data":           eventData,
			"ts_ms":          env.GetTsMs(),
		})
		if err == nil {
			c.cfg.Publisher.Publish(run.AppID, env.GetRunId(), wsPayload)
		}
	}
	if event.GetType() != runtimev1.EventType_RUN_STATUS {
		return
	}
	statusValue, _ := eventData[contract.EventFieldStatus].(string)
	status := strings.ToLower(statusValue)
	switch status {
	case "running", "waiting", "completed", "failed", "cancelled", "timeout":
	default:
		return
	}
	if status == run.Status {
		return
	}
	if status == runstore.StatusWaiting {
		// Serialize the waiting transition and task cancellation with resume.
		// Otherwise a resume could enqueue work between the transition and
		// cancellation and immediately lose that work.
		c.waitingMu.Lock()
		defer c.waitingMu.Unlock()
	}
	rejectedWaiting := status == runstore.StatusWaiting &&
		(run.Runtime != "longterm" || run.SessionID == "")
	rejectedReason := "waiting requires a longterm session"
	if status == runstore.StatusWaiting && !rejectedWaiting && c.cfg.Checkpoints != nil {
		checkpoint, err := c.resumeCheckpoint(ctx, run)
		if err != nil || checkpoint == nil {
			rejectedWaiting = true
			rejectedReason = "waiting requires a saved checkpoint"
		}
	}
	if rejectedWaiting {
		c.log.Warn("control rejected waiting status",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldTenantID, run.TenantID,
			"runtime", run.Runtime,
			"session_id", run.SessionID,
			"reason", rejectedReason,
		)
		status = runstore.StatusFailed
	}
	if _, err := c.runs.Transition(ctx, runstore.Transition{
		RunID:              env.GetRunId(),
		ExpectedStatus:     run.Status,
		ToStatus:           status,
		ExpectedAttempt:    int(env.GetAttempt()),
		ExpectedLeaseEpoch: env.GetLeaseEpoch(),
	}); err != nil {
		if !errors.Is(err, runstore.ErrRunNotAssignable) {
			c.log.Warn("control transition run failed",
				"run_id", env.GetRunId(),
				"status", status,
				"err", err,
			)
			return
		}
		// The run was requeued or already finished while this event was in
		// flight. Never publish a terminal state that the store rejected.
		c.log.Info("control run terminal superseded",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldTenantID, run.TenantID,
			observe.FieldAttempt, env.GetAttempt(),
			observe.FieldStatus, status,
			"current_status", run.Status,
		)
		return
	}
	if status == runstore.StatusWaiting {
		c.transport.Release(run.AssignedDaemonID, run.RunID)
		c.cancelTasks(ctx, env.GetRunId(), "run waiting")
		c.log.Info("control run waiting",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldTenantID, run.TenantID,
			observe.FieldAttempt, env.GetAttempt(),
		)
		return
	}
	if runstore.TerminalStatus(status) {
		c.log.Info("control run terminal",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldTenantID, run.TenantID,
			observe.FieldAttempt, env.GetAttempt(),
			observe.FieldStatus, status,
		)
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricRunsTotal, "Terminal runs by status, runtime, and tenant.", map[string]string{
				"status": status, "runtime": run.Runtime, "tenant": run.TenantID,
			})
		}
		c.transport.Release(run.AssignedDaemonID, run.RunID)
		if status == runstore.StatusCancelled {
			c.cancelLocalTools(run.AppID, env.GetRunId(), string(errs.CodeRunCancelled), "run cancelled")
		}
		if rejectedWaiting {
			c.appendMWStatus(ctx, run, runstore.StatusFailed, rejectedReason)
		}
		c.cancelTasks(ctx, env.GetRunId(), "run terminal")
		c.publishCompleted(run.AppID, env.GetRunId(), status, int(env.GetAttempt()))
	}
}

// StaleEvents returns the number of dropped stale-epoch events.
func (c *Control) StaleEvents() int64 { return c.staleEvents.Load() }

// GapEvents returns the number of events that exceeded the reorder window.
func (c *Control) GapEvents() int64 { return c.gapEvents.Load() }

// EphemeralEvents returns the number of skipped ephemeral events.
func (c *Control) EphemeralEvents() int64 { return c.ephemeralEvents.Load() }

// CriticalEvents returns the number of critical events lost to a full buffer.
func (c *Control) CriticalEvents() int64 { return c.criticalEvents.Load() }

func firstNonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (c *Control) observeUsage(run runstore.Run, event *runtimev1.Event, data map[string]any) {
	if c.metrics == nil || event.GetType() != runtimev1.EventType_USAGE {
		return
	}
	input, _ := data[contract.EventFieldInputTokens].(float64)
	output, _ := data[contract.EventFieldOutputTokens].(float64)
	if input == 0 && output == 0 {
		return
	}
	model := "unknown"
	var profile struct {
		Model string `json:"model"`
	}
	if len(run.Model) > 0 && json.Unmarshal(run.Model, &profile) == nil && profile.Model != "" {
		model = profile.Model
	}
	c.metrics.Add(observe.MetricTokenUsageTotal, "Token usage from USAGE events.", map[string]string{
		"app": run.AppID, "model": model, "tenant": run.TenantID,
	}, input+output)
}
