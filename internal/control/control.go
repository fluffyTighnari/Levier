// Middleware — internal/control
//
// Minimal run scheduler and lifecycle driver.
//
// Copyright 2026 The Middleware Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/observe"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/register"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

var (
	errNoDaemon   = errors.New("no healthy daemon available")
	errAckTimeout = errors.New("task ack timed out")
)

// EventPublisher pushes events to App WebSocket subscribers.
type EventPublisher interface {
	Publish(appID, runID string, payload []byte)
}

// Config controls the minimal Control loop.
type Config struct {
	AckTimeout           time.Duration
	RetryDelay           time.Duration
	MaxPlacementAttempts int
	EventBuffer          int
	Publisher            EventPublisher
	Scheduler            Scheduler
	Tools                ToolInvoker
	Tasks                TaskStore
	LocalTools           LocalToolCanceller
	Logger               *slog.Logger
	Metrics              *observe.Registry
}

// ToolInvoker routes one ToolCallRequest.
type ToolInvoker interface {
	Invoke(ctx context.Context, runID, appID string, request *runtimev1.ToolCallRequest) *runtimev1.ToolCallResult
}

// TaskStore cancels and enqueues durable RunTasks.
type TaskStore interface {
	CancelRun(ctx context.Context, runID, reason string) (int, error)
	Enqueue(ctx context.Context, task workflow.Task) (workflow.Task, error)
}

// LocalToolCanceller fails pending App tool calls for a run.
type LocalToolCanceller interface {
	CancelLocalTools(appID, runID, code, message string)
}

// RunStore is the persistence subset used by Control.
type RunStore interface {
	GetRun(ctx context.Context, runID string) (runstore.Run, error)
	AssignRun(ctx context.Context, assignment runstore.Assignment) (runstore.Run, error)
	Transition(ctx context.Context, transition runstore.Transition) (runstore.Run, error)
	AppendEvent(ctx context.Context, event runstore.Event) (runstore.AppendResult, error)
	ListActiveRunsForDaemon(ctx context.Context, daemonID string) ([]runstore.Run, error)
	RequeueRun(ctx context.Context, runID string) (runstore.Run, error)
}

// MaterialAssembler supplies TaskAssign tools and MCP bindings.
type MaterialAssembler interface {
	AssembleTaskMaterials(ctx context.Context, appID string) (register.TaskMaterials, error)
}

// DaemonHub is the connection subset used by Control.
type DaemonHub interface {
	PickHealthy(runtimeType runtimev1.RuntimeType, strategy string, labels map[string]string) (string, bool)
	Reserve(daemonID string) bool
	Release(daemonID string)
	Send(daemonID string, env *runtimev1.Envelope) error
}

// Control claims RUN_DISPATCH tasks, assigns daemons, and drives run state.
type Control struct {
	runs      RunStore
	hub       DaemonHub
	materials MaterialAssembler
	cfg       Config
	log       *slog.Logger
	metrics   *observe.Registry

	pendingMu sync.Mutex
	pending   map[string]chan *runtimev1.TaskAck
	events    chan *runtimev1.Envelope

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	staleEvents     atomic.Int64
	gapEvents       atomic.Int64
	ephemeralEvents atomic.Int64
}

// New returns a Control instance.
func New(runs RunStore, hub DaemonHub, cfg Config, assemblers ...MaterialAssembler) *Control {
	if cfg.AckTimeout <= 0 {
		cfg.AckTimeout = 5 * time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	if cfg.MaxPlacementAttempts <= 0 {
		cfg.MaxPlacementAttempts = 3
	}
	if cfg.EventBuffer <= 0 {
		cfg.EventBuffer = 256
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	control := &Control{
		runs:    runs,
		hub:     hub,
		cfg:     cfg,
		log:     cfg.Logger,
		metrics: cfg.Metrics,
		pending: make(map[string]chan *runtimev1.TaskAck),
		events:  make(chan *runtimev1.Envelope, cfg.EventBuffer),
	}
	if len(assemblers) > 0 {
		control.materials = assemblers[0]
	}
	return control
}

// Start starts the asynchronous event consumer.
func (c *Control) Start(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go c.eventLoop()
}

// Close stops the event consumer.
func (c *Control) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// HandleToolCall executes one daemon ToolCallRequest through the gateway.
func (c *Control) HandleToolCall(daemonID string, env *runtimev1.Envelope) error {
	if c.cfg.Tools == nil || env == nil || env.GetToolCallRequest() == nil {
		return nil
	}
	ctx := context.Background()
	run, err := c.runs.GetRun(ctx, env.GetRunId())
	if err != nil {
		return fmt.Errorf("get run for tool call: %w", err)
	}
	result := c.cfg.Tools.Invoke(ctx, env.GetRunId(), run.AppID, env.GetToolCallRequest())
	response, err := protocol.NewResponse(env, result)
	if err != nil {
		return fmt.Errorf("build tool call result: %w", err)
	}
	if err := c.hub.Send(daemonID, response); err != nil {
		return fmt.Errorf("send tool call result: %w", err)
	}
	return nil
}

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

// Handle implements workflow.Handler for one claimed task.
func (c *Control) Handle(ctx context.Context, task workflow.Task, _ workflow.Lease) error {
	if task.Type == workflow.TaskTimeout {
		return c.handleTimeout(ctx, task.RunID)
	}
	if task.Type != workflow.TaskDispatch && task.Type != workflow.TaskRetry {
		return nil
	}

	run, err := c.runs.GetRun(ctx, task.RunID)
	if errors.Is(err, runstore.ErrRunNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	switch run.Status {
	case "completed", "failed", "cancelled", "timeout", "running":
		return nil
	}

	attempt := run.Attempt + 1
	leaseEpoch := run.LeaseEpoch + 1
	assign, err := buildTaskAssign(run, attempt)
	if err != nil {
		return workflow.Permanent(err)
	}
	if c.materials != nil {
		materials, err := c.materials.AssembleTaskMaterials(ctx, run.AppID)
		if err != nil {
			return workflow.Permanent(fmt.Errorf("assemble task materials: %w", err))
		}
		assign.Tools = toolManifests(materials.Tools)
		assign.McpBindings = mcpBindings(materials.MCPBindings)
	}

	placement := PlacementRequest{
		Runtime:  runtimeType(run.Runtime),
		Strategy: run.Strategy,
	}
	if run.Runtime == "longterm" {
		placement.StickyDaemonID = run.AssignedDaemonID
	}
	excluded := make(map[string]struct{})
	status := run.Status
	currentAttempt := run.Attempt
	currentEpoch := run.LeaseEpoch
	lastErr := fmt.Errorf("%w: no eligible daemon", errNoDaemon)

	for attemptIndex := 0; attemptIndex < c.cfg.MaxPlacementAttempts; attemptIndex++ {
		placement.ExcludeDaemonIDs = excluded
		daemonID, ok := c.pick(placement)
		if !ok {
			break
		}
		if !c.hub.Reserve(daemonID) {
			excluded[daemonID] = struct{}{}
			continue
		}

		assigned, err := c.runs.AssignRun(ctx, runstore.Assignment{
			RunID:              run.RunID,
			ExpectedStatus:     status,
			ExpectedAttempt:    currentAttempt,
			ExpectedLeaseEpoch: currentEpoch,
			Attempt:            attempt,
			LeaseEpoch:         leaseEpoch,
			DaemonID:           daemonID,
		})
		if err != nil {
			c.hub.Release(daemonID)
			if errors.Is(err, runstore.ErrRunNotAssignable) {
				return nil
			}
			return fmt.Errorf("assign run: %w", err)
		}
		status = assigned.Status
		currentAttempt = assigned.Attempt
		currentEpoch = assigned.LeaseEpoch
		if c.metrics != nil && assigned.AssignedAt != nil && !assigned.CreatedAt.IsZero() {
			c.metrics.Observe(observe.MetricRunQueueDuration, "Run queue duration in seconds.", map[string]string{
				"runtime": run.Runtime,
			}, assigned.AssignedAt.Sub(assigned.CreatedAt).Seconds())
		}
		c.log.Info("control assigned run",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldAttempt, attempt,
			observe.FieldLeaseEpoch, leaseEpoch,
			observe.FieldDaemonID, daemonID,
			"runtime", run.Runtime,
			"strategy", run.Strategy,
		)

		envelope, err := protocol.NewRun(run.RunID, int32(attempt), leaseEpoch, assign)
		if err != nil {
			c.hub.Release(daemonID)
			return workflow.Permanent(fmt.Errorf("build task assign: %w", err))
		}
		ackCh := make(chan *runtimev1.TaskAck, 1)
		c.pendingMu.Lock()
		c.pending[envelope.GetMsgId()] = ackCh
		c.pendingMu.Unlock()

		if err := c.hub.Send(daemonID, envelope); err != nil {
			c.removePending(envelope.GetMsgId())
			c.hub.Release(daemonID)
			return fmt.Errorf("send task assign: %w", err)
		}

		timer := time.NewTimer(c.cfg.AckTimeout)
		select {
		case <-ctx.Done():
			timer.Stop()
			c.removePending(envelope.GetMsgId())
			c.hub.Release(daemonID)
			return ctx.Err()
		case <-timer.C:
			c.removePending(envelope.GetMsgId())
			c.hub.Release(daemonID)
			lastErr = errAckTimeout
			if c.cfg.Scheduler == nil {
				return errAckTimeout
			}
			excluded[daemonID] = struct{}{}
			attempt++
			leaseEpoch++
			continue
		case ack := <-ackCh:
			timer.Stop()
			c.removePending(envelope.GetMsgId())
			if ack.GetAccepted() {
				return nil
			}
			c.hub.Release(daemonID)
			excluded[daemonID] = struct{}{}
			lastErr = fmt.Errorf("daemon %q rejected task: %s", daemonID, ack.GetReason())
			if c.cfg.Scheduler == nil {
				return lastErr
			}
			attempt++
			leaseEpoch++
		}
	}
	return lastErr
}

func (c *Control) pick(placement PlacementRequest) (string, bool) {
	if c.cfg.Scheduler != nil {
		return c.cfg.Scheduler.Pick(placement)
	}
	return c.hub.PickHealthy(placement.Runtime, placement.Strategy, placement.Labels)
}

func (c *Control) removePending(msgID string) {
	c.pendingMu.Lock()
	delete(c.pending, msgID)
	c.pendingMu.Unlock()
}

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

// ReconcileDaemon compares Hello.running_runs with persisted run state.
func (c *Control) ReconcileDaemon(info connection.DaemonInfo) {
	ctx := context.Background()
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
	for _, run := range runs {
		if runstore.TerminalStatus(run.Status) {
			if _, ok := running[run.RunID]; ok {
				cancelIDs = append(cancelIDs, run.RunID)
				delete(running, run.RunID)
			}
			continue
		}
		reported, ok := running[run.RunID]
		if !ok {
			if run.Status == runstore.StatusRunning {
				c.requeueRun(ctx, run, "daemon lost run")
			}
			continue
		}
		if reported.LeaseEpoch != run.LeaseEpoch {
			cancelIDs = append(cancelIDs, run.RunID)
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
			return
		}
		if err := c.hub.Send(info.DaemonID, envelope); err != nil {
			c.log.Warn("control send reconcile failed", "daemon_id", info.DaemonID, "err", err)
		}
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
	c.hub.Release(run.AssignedDaemonID)
	c.cancelLocalTools(run.AppID, run.RunID, "DAEMON_LOST", reason)
	if c.cfg.Tasks != nil {
		if _, err := c.cfg.Tasks.Enqueue(ctx, workflow.NewTask(
			requeued.RunID, workflow.TaskRetry, time.Now(), 0,
		)); err != nil {
			c.log.Warn("control enqueue retry failed", "run_id", run.RunID, "err", err)
		}
	}
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
	c.sendCancel(ctx, run, reason)
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
	c.hub.Release(run.AssignedDaemonID)
	c.cancelTasks(ctx, runID, reason)
	c.cancelLocalTools(run.AppID, runID, "RUN_CANCELLED", reason)
	c.publishCompleted(run.AppID, runID, runstore.StatusCancelled, run.Attempt)
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
	if runstore.TerminalStatus(run.Status) {
		return nil
	}
	c.sendCancel(ctx, run, "timeout")
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
	c.hub.Release(run.AssignedDaemonID)
	c.cancelLocalTools(run.AppID, runID, "RUN_TIMEOUT", "run timed out")
	c.publishCompleted(run.AppID, runID, runstore.StatusTimeout, timedOut.Attempt)
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

func (c *Control) publishCompleted(appID, runID, status string, attempt int) {
	if c.cfg.Publisher == nil || appID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"type": "run_completed", "run_id": runID, "status": status, "attempt": attempt,
	})
	c.cfg.Publisher.Publish(appID, runID, payload)
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
	}); err != nil && !errors.Is(err, runstore.ErrRunNotAssignable) {
		c.log.Warn("control transition run failed",
			"run_id", env.GetRunId(),
			"status", status,
			"err", err,
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
			c.cancelLocalTools(run.AppID, env.GetRunId(), "RUN_CANCELLED", "run cancelled")
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

func buildTaskAssign(run runstore.Run, attempt int) (*runtimev1.TaskAssign, error) {
	params := map[string]any{}
	if len(run.Params) > 0 {
		if err := json.Unmarshal(run.Params, &params); err != nil {
			return nil, fmt.Errorf("decode run params: %w", err)
		}
	}
	strategyParams, err := structpb.NewStruct(params)
	if err != nil {
		return nil, fmt.Errorf("build strategy params: %w", err)
	}

	var input []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Name    string `json:"name"`
	}
	if len(run.Input) > 0 {
		if err := json.Unmarshal(run.Input, &input); err != nil {
			return nil, fmt.Errorf("decode run input: %w", err)
		}
	}
	messages := make([]*runtimev1.ChatMessage, 0, len(input))
	for _, message := range input {
		if message.Role == "" {
			return nil, fmt.Errorf("run input contains empty role")
		}
		messages = append(messages, &runtimev1.ChatMessage{
			Role:    message.Role,
			Content: message.Content,
			Name:    message.Name,
		})
	}

	var model struct {
		Provider string            `json:"provider"`
		Model    string            `json:"model"`
		APIKey   string            `json:"api_key"`
		BaseURL  string            `json:"base_url"`
		Params   map[string]string `json:"params"`
	}
	if len(run.Model) > 0 {
		if err := json.Unmarshal(run.Model, &model); err != nil {
			return nil, fmt.Errorf("decode run model: %w", err)
		}
	}

	runTimeout := int64(run.RunTimeoutMS)
	if runTimeout <= 0 {
		runTimeout = 1_800_000
	}
	toolTimeout := int64(run.ToolTimeoutMS)
	if toolTimeout <= 0 {
		toolTimeout = 60_000
	}
	return &runtimev1.TaskAssign{
		RunId:          run.RunID,
		AppId:          run.AppID,
		Strategy:       run.Strategy,
		StrategyParams: strategyParams,
		Input:          messages,
		Model: &runtimev1.ModelProfile{
			Provider: model.Provider,
			Model:    model.Model,
			ApiKey:   model.APIKey,
			BaseUrl:  model.BaseURL,
			Params:   model.Params,
		},
		Timeouts: &runtimev1.TimeoutProfile{
			RunTimeoutMs:  runTimeout,
			ToolTimeoutMs: toolTimeout,
		},
		SessionId:    run.SessionID,
		TraceContext: run.TraceID,
	}, nil
}

func toolManifests(tools []register.ToolMaterial) []*runtimev1.ToolManifest {
	result := make([]*runtimev1.ToolManifest, 0, len(tools))
	for _, tool := range tools {
		scope := runtimev1.ToolScope_GLOBAL
		if tool.Scope == "local" {
			scope = runtimev1.ToolScope_LOCAL
		}
		result = append(result, &runtimev1.ToolManifest{
			Name:            tool.Name,
			Description:     tool.Description,
			InputSchemaJson: tool.SchemaJSON,
			Scope:           scope,
		})
	}
	return result
}

func mcpBindings(bindings []register.MCPMaterial) []*runtimev1.McpBinding {
	result := make([]*runtimev1.McpBinding, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, &runtimev1.McpBinding{
			ServerName:   binding.ServerName,
			Transport:    binding.Transport,
			Command:      binding.Command,
			Args:         binding.Args,
			Env:          binding.Env,
			Url:          binding.URL,
			Headers:      binding.Headers,
			AllowedTools: binding.AllowedTools,
			Namespace:    binding.Namespace,
		})
	}
	return result
}

func runtimeType(value string) runtimev1.RuntimeType {
	switch strings.ToLower(value) {
	case "normal":
		return runtimev1.RuntimeType_NORMAL
	case "sandbox":
		return runtimev1.RuntimeType_SANDBOX
	case "longterm":
		return runtimev1.RuntimeType_LONGTERM
	default:
		return runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED
	}
}

// Ensure Control satisfies the worker handler contract.
var _ workflow.Handler = (*Control)(nil)
