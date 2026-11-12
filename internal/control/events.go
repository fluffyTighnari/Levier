// Middleware — control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"strings"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/runstore"
)

// HandleEnvelope routes TaskAck and Event messages from DaemonHub.
func (c *Control) HandleEnvelope(_ string, env *runtimev1.Envelope) {
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
	if env.GetEvent() != nil {
		select {
		case c.events <- env:
		default:
			if c.metrics != nil {
				c.metrics.Inc(observe.MetricEventsDroppedTotal, "Dropped or skipped events by reason.", map[string]string{
					"reason": "control_buffer_full",
				})
			}
			c.log.Warn("control event buffer full", "run_id", env.GetRunId())
		}
	}
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
	result, err := c.runs.AppendEvent(ctx, runstore.Event{
		RunID:      env.GetRunId(),
		Attempt:    int(env.GetAttempt()),
		Seq:        event.GetSeq(),
		Source:     source,
		Type:       runtimev1.EventType_name[int32(event.GetType())],
		Payload:    payload,
		TSMs:       env.GetTsMs(),
		LeaseEpoch: env.GetLeaseEpoch(),
	})
	if err != nil {
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
			"type": runtimev1.EventType_name[int32(event.GetType())],
		})
		c.observeUsage(run, event, eventData)
	}
	if c.cfg.Publisher != nil {
		wsPayload, err := json.Marshal(map[string]any{
			"type":       "event",
			"run_id":     env.GetRunId(),
			"attempt":    env.GetAttempt(),
			"seq":        event.GetSeq(),
			"source":     source,
			"event_type": runtimev1.EventType_name[int32(event.GetType())],
			"data":       eventData,
			"ts_ms":      env.GetTsMs(),
		})
		if err == nil {
			c.cfg.Publisher.Publish(run.AppID, env.GetRunId(), wsPayload)
		}
	}
	if event.GetType() != runtimev1.EventType_RUN_STATUS {
		return
	}
	statusValue, _ := eventData["status"].(string)
	status := strings.ToLower(statusValue)
	switch status {
	case "running", "completed", "failed", "cancelled", "timeout":
	default:
		return
	}
	if status == run.Status {
		return
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
			observe.FieldAttempt, env.GetAttempt(),
			observe.FieldStatus, status,
			"current_status", run.Status,
		)
		return
	}
	if runstore.TerminalStatus(status) {
		c.log.Info("control run terminal",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldAttempt, env.GetAttempt(),
			observe.FieldStatus, status,
		)
		if c.metrics != nil {
			c.metrics.Inc(observe.MetricRunsTotal, "Terminal runs by status and runtime.", map[string]string{
				"status": status, "runtime": run.Runtime,
			})
		}
		c.hub.Release(run.AssignedDaemonID)
		if status == runstore.StatusCancelled {
			c.cancelLocalTools(run.AppID, env.GetRunId(), string(errs.CodeRunCancelled), "run cancelled")
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

func (c *Control) observeUsage(run runstore.Run, event *runtimev1.Event, data map[string]any) {
	if c.metrics == nil || event.GetType() != runtimev1.EventType_USAGE {
		return
	}
	input, _ := data["input_tokens"].(float64)
	output, _ := data["output_tokens"].(float64)
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
		"app": run.AppID, "model": model,
	}, input+output)
}
