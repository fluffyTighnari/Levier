// Levier — control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/register"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/workflow"
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
	CriticalEventWait    time.Duration
	ToolConcurrency      int
	Publisher            EventPublisher
	Scheduler            Scheduler
	Tools                ToolInvoker
	Tasks                TaskStore
	LocalTools           LocalToolCanceller
	ModelProviders       ModelProviderResolver
	Checkpoints          CheckpointStore
	DaemonAvailable      func(daemonID string) bool
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

// CheckpointStore persists and loads longterm session state.
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, checkpoint runstore.Checkpoint) error
	GetCheckpoint(ctx context.Context, sessionID, appID, tenantID string) (runstore.Checkpoint, error)
}

// LocalToolCanceller fails pending App tool calls for a run.
type LocalToolCanceller interface {
	CancelLocalTools(appID, runID, code, message string)
}

// RunStore is the persistence subset used by Control.
type RunStore interface {
	GetRun(ctx context.Context, runID string) (runstore.Run, error)
	ListQueuedRuns(ctx context.Context) ([]runstore.Run, error)
	AssignRun(ctx context.Context, assignment runstore.Assignment) (runstore.Run, error)
	Transition(ctx context.Context, transition runstore.Transition) (runstore.Run, error)
	AppendEvent(ctx context.Context, event runstore.Event) (runstore.AppendResult, error)
	AppendMWEvent(ctx context.Context, event runstore.Event) (runstore.Event, error)
	ListActiveRuns(ctx context.Context) ([]runstore.Run, error)
	ListActiveRunsForDaemon(ctx context.Context, daemonID string) ([]runstore.Run, error)
	RequeueRun(ctx context.Context, runID string) (runstore.Run, error)
	RequeueRunWithResume(ctx context.Context, runID string, resume json.RawMessage) (runstore.Run, error)
}

// MaterialAssembler supplies TaskAssign tools and MCP bindings.
type MaterialAssembler interface {
	AssembleTaskMaterials(ctx context.Context, appID string) (register.TaskMaterials, error)
}

// ModelProviderResolver resolves a logical provider/model into runtime settings.
type ModelProviderResolver interface {
	ResolveModelProfile(ctx context.Context, name, model string, params map[string]string) (register.ModelProfile, error)
	ResolveDefaultModelProfile(ctx context.Context, params map[string]string) (register.ModelProfile, error)
}

// DaemonHub is the connection subset used by Control.
type DaemonHub interface {
	PickHealthy(runtimeType runtimev1.RuntimeType, runtimeName string, labels map[string]string) (string, bool)
	Reserve(daemonID string) bool
	Release(daemonID string)
	Send(daemonID string, env *runtimev1.Envelope) error
	Disconnect(daemonID string)
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
	waitingMu sync.Mutex
	events    chan *runtimev1.Envelope

	toolMu     sync.Mutex
	toolClosed bool
	toolSlots  chan struct{}
	toolWG     sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	staleEvents     atomic.Int64
	gapEvents       atomic.Int64
	ephemeralEvents atomic.Int64
	criticalEvents  atomic.Int64
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
	if cfg.CriticalEventWait <= 0 {
		cfg.CriticalEventWait = 500 * time.Millisecond
	}
	if cfg.ToolConcurrency <= 0 {
		cfg.ToolConcurrency = 64
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	control := &Control{
		runs:      runs,
		hub:       hub,
		cfg:       cfg,
		log:       cfg.Logger,
		metrics:   cfg.Metrics,
		pending:   make(map[string]chan *runtimev1.TaskAck),
		events:    make(chan *runtimev1.Envelope, cfg.EventBuffer),
		toolSlots: make(chan struct{}, cfg.ToolConcurrency),
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

// Close stops the event consumer and waits for in-flight tool calls.
func (c *Control) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	c.toolMu.Lock()
	c.toolClosed = true
	c.toolMu.Unlock()
	c.toolWG.Wait()
	c.wg.Wait()
}

// Ensure Control satisfies the worker handler contract.
var _ workflow.Handler = (*Control)(nil)
