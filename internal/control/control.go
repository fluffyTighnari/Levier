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
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

var (
	errNoDaemon   = errors.New("no healthy daemon available")
	errAckTimeout = errors.New("task ack timed out")
)

// Config controls the minimal Control loop.
type Config struct {
	AckTimeout  time.Duration
	RetryDelay  time.Duration
	EventBuffer int
	Logger      *slog.Logger
}

// RunStore is the persistence subset used by Control.
type RunStore interface {
	GetRun(ctx context.Context, runID string) (runstore.Run, error)
	AssignRun(ctx context.Context, assignment runstore.Assignment) (runstore.Run, error)
	Transition(ctx context.Context, transition runstore.Transition) (runstore.Run, error)
	AppendEvent(ctx context.Context, event runstore.Event) (bool, error)
}

// DaemonHub is the connection subset used by Control.
type DaemonHub interface {
	PickHealthy(runtimeType runtimev1.RuntimeType, strategy string, labels map[string]string) (string, bool)
	Send(daemonID string, env *runtimev1.Envelope) error
}

// Control claims RUN_DISPATCH tasks, assigns daemons, and drives run state.
type Control struct {
	runs RunStore
	hub  DaemonHub
	cfg  Config
	log  *slog.Logger

	pendingMu sync.Mutex
	pending   map[string]chan *runtimev1.TaskAck
	events    chan *runtimev1.Envelope

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New returns a Control instance.
func New(runs RunStore, hub DaemonHub, cfg Config) *Control {
	if cfg.AckTimeout <= 0 {
		cfg.AckTimeout = 5 * time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	if cfg.EventBuffer <= 0 {
		cfg.EventBuffer = 256
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Control{
		runs:    runs,
		hub:     hub,
		cfg:     cfg,
		log:     cfg.Logger,
		pending: make(map[string]chan *runtimev1.TaskAck),
		events:  make(chan *runtimev1.Envelope, cfg.EventBuffer),
	}
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
			c.log.Warn("control event buffer full", "run_id", env.GetRunId())
		}
	}
}

// Handle implements workflow.Handler for one claimed task.
func (c *Control) Handle(ctx context.Context, task workflow.Task, _ workflow.Lease) error {
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
	case "completed", "failed", "cancelled", "timeout":
		return nil
	case "running":
		return nil
	}

	attempt := run.Attempt + 1
	leaseEpoch := run.LeaseEpoch + 1
	daemonID, ok := c.hub.PickHealthy(runtimeType(run.Runtime), run.Strategy, nil)
	if !ok {
		return workflow.RetryAfter(errNoDaemon, c.cfg.RetryDelay)
	}

	assign, err := buildTaskAssign(run, attempt)
	if err != nil {
		return workflow.Permanent(err)
	}
	envelope, err := protocol.NewRun(run.RunID, int32(attempt), leaseEpoch, assign)
	if err != nil {
		return workflow.Permanent(fmt.Errorf("build task assign: %w", err))
	}

	if _, err := c.runs.AssignRun(ctx, runstore.Assignment{
		RunID:              run.RunID,
		ExpectedStatus:     run.Status,
		ExpectedAttempt:    run.Attempt,
		ExpectedLeaseEpoch: run.LeaseEpoch,
		Attempt:            attempt,
		LeaseEpoch:         leaseEpoch,
		DaemonID:           daemonID,
	}); err != nil {
		if errors.Is(err, runstore.ErrRunNotAssignable) {
			return nil
		}
		return fmt.Errorf("assign run: %w", err)
	}

	ackCh := make(chan *runtimev1.TaskAck, 1)
	c.pendingMu.Lock()
	c.pending[envelope.GetMsgId()] = ackCh
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, envelope.GetMsgId())
		c.pendingMu.Unlock()
	}()

	if err := c.hub.Send(daemonID, envelope); err != nil {
		return workflow.RetryAfter(fmt.Errorf("send task assign: %w", err), c.cfg.RetryDelay)
	}

	timer := time.NewTimer(c.cfg.AckTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return workflow.RetryAfter(errAckTimeout, c.cfg.RetryDelay)
	case ack := <-ackCh:
		if !ack.GetAccepted() {
			reason := ack.GetReason()
			if reason == "" {
				reason = "daemon rejected task"
			}
			return workflow.RetryAfter(errors.New(reason), c.cfg.RetryDelay)
		}
		return nil
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
	ctx := context.Background()
	run, err := c.runs.GetRun(ctx, env.GetRunId())
	if errors.Is(err, runstore.ErrRunNotFound) {
		return
	}
	if err != nil {
		c.log.Warn("control get run failed", "run_id", env.GetRunId(), "err", err)
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
	inserted, err := c.runs.AppendEvent(ctx, runstore.Event{
		RunID:   env.GetRunId(),
		Attempt: int(env.GetAttempt()),
		Seq:     event.GetSeq(),
		Source:  source,
		Type:    runtimev1.EventType_name[int32(event.GetType())],
		Payload: payload,
		TSMs:    env.GetTsMs(),
	})
	if err != nil {
		c.log.Warn("control append event failed", "run_id", env.GetRunId(), "err", err)
		return
	}
	if !inserted || event.GetType() != runtimev1.EventType_RUN_STATUS {
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
	}
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
