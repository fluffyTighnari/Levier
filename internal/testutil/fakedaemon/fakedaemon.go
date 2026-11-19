// Levier — internal/testutil/fakedaemon
//
// Scriptable in-process daemon client used by middleware tests.
//
// Copyright 2026 The Levier Authors.
package fakedaemon

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// Config controls the fake daemon's default behavior.
type Config struct {
	DaemonID                   string
	AuthToken                  string
	RuntimeName                string
	RuntimeType                runtimev1.RuntimeType
	MaxConcurrentRuns          int32
	Version                    string
	Labels                     map[string]string
	AutoEcho                   bool
	Accept                     bool
	RejectReason               string
	AckDelay                   time.Duration
	SkipHello                  bool
	HeartbeatInterval          time.Duration
	DisconnectAfterAssignments int
}

// DefaultConfig returns a valid fake daemon configuration.
func DefaultConfig() Config {
	return Config{
		DaemonID:          "fake-daemon",
		AuthToken:         "test-token",
		RuntimeName:       "fake1",
		RuntimeType:       runtimev1.RuntimeType_NORMAL,
		MaxConcurrentRuns: 4,
		Version:           "test",
		Labels:            map[string]string{"pool": "test"},
		Accept:            true,
	}
}

// Daemon is a scriptable daemon-side RuntimeChannel client.
type Daemon struct {
	cfg    Config
	stream runtimev1.RuntimeService_ChannelClient
	cancel context.CancelFunc
	conn   *grpc.ClientConn

	sendMu  sync.Mutex
	pending sync.Map

	assignments chan *runtimev1.Envelope
	events      chan *runtimev1.Envelope
	errCh       chan error
	seqMu       sync.Mutex
	seq         map[string]int64
	running     atomic.Int32

	onMessage func(context.Context, *runtimev1.Envelope, *Daemon) error
}

// New opens a RuntimeChannel on conn and starts the receive loop.
func New(ctx context.Context, conn grpc.ClientConnInterface, cfg Config) (*Daemon, error) {
	cfg = normalize(cfg)
	streamCtx, cancel := context.WithCancel(ctx)
	client := runtimev1.NewRuntimeServiceClient(conn)
	stream, err := client.Channel(streamCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open runtime channel: %w", err)
	}

	d := &Daemon{
		cfg:         cfg,
		stream:      stream,
		cancel:      cancel,
		assignments: make(chan *runtimev1.Envelope, 32),
		events:      make(chan *runtimev1.Envelope, 128),
		errCh:       make(chan error, 1),
		seq:         make(map[string]int64),
	}

	if !cfg.SkipHello {
		if err := d.sendHello(); err != nil {
			cancel()
			return nil, err
		}
	}
	if cfg.HeartbeatInterval > 0 {
		go d.heartbeatLoop(streamCtx)
	}
	go d.recvLoop(streamCtx)
	return d, nil
}

// Dial connects to a gRPC target and returns a fake daemon.
func Dial(ctx context.Context, target string, cfg Config) (*Daemon, error) {
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	d, err := New(ctx, conn, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	d.conn = conn
	return d, nil
}

// Send validates and sends one envelope.
func (d *Daemon) Send(env *runtimev1.Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	if err := protocol.Validate(env); err != nil {
		return err
	}
	d.sendMu.Lock()
	defer d.sendMu.Unlock()
	if err := d.stream.Send(env); err != nil {
		return fmt.Errorf("send envelope: %w", err)
	}
	if env.GetEvent() != nil {
		select {
		case d.events <- env:
		default:
		}
	}
	return nil
}

// Assignments returns envelopes containing TaskAssign.
func (d *Daemon) Assignments() <-chan *runtimev1.Envelope {
	return d.assignments
}

// Events returns outbound events observed locally. It is populated by Send.
func (d *Daemon) Events() <-chan *runtimev1.Envelope {
	return d.events
}

// Err returns the receive-loop error, if any.
func (d *Daemon) Err() error {
	select {
	case err := <-d.errCh:
		return err
	default:
		return nil
	}
}

// SetOnMessage installs a handler for non-ToolCallResult inbound messages.
func (d *Daemon) SetOnMessage(handler func(context.Context, *runtimev1.Envelope, *Daemon) error) {
	d.onMessage = handler
}

// Close closes the stream and any owned client connection.
func (d *Daemon) Close() error {
	d.cancel()
	if d.conn != nil {
		return d.conn.Close()
	}
	return nil
}

// WaitReady gives the server a short grace period to reject the hello.
//
// The RuntimeChannel protocol has no explicit hello acknowledgement, so a
// successful channel is observed by the absence of an early stream error.
func (d *Daemon) WaitReady(grace time.Duration) error {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-d.errCh:
		return err
	case <-timer.C:
		return nil
	}
}

// RequestTool sends a ToolCallRequest and waits for the matching result.
func (d *Daemon) RequestTool(ctx context.Context, runID string, attempt int32, leaseEpoch int64, toolName, argumentsJSON string, timeoutMS int64) (*runtimev1.ToolCallResult, error) {
	invocationID := id.New()
	req := &runtimev1.ToolCallRequest{
		InvocationId:  invocationID,
		ToolName:      toolName,
		ArgumentsJson: argumentsJSON,
		TimeoutMs:     timeoutMS,
	}
	env, err := protocol.NewRun(runID, attempt, leaseEpoch, req)
	if err != nil {
		return nil, err
	}

	resultCh := make(chan *runtimev1.ToolCallResult, 1)
	d.pending.Store(invocationID, resultCh)
	defer d.pending.Delete(invocationID)

	if err := d.Send(env); err != nil {
		return nil, err
	}

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendEvent sends one persistent daemon event with the next sequence number.
func (d *Daemon) SendEvent(runID string, attempt int32, leaseEpoch int64, eventType runtimev1.EventType, data map[string]any) error {
	value, err := structpb.NewStruct(data)
	if err != nil {
		return fmt.Errorf("build event data: %w", err)
	}
	event := &runtimev1.Event{
		Seq:    d.nextSeq(runID),
		Type:   eventType,
		Source: protocol.EventSourceDaemon,
		Data:   value,
	}
	env, err := protocol.NewRun(runID, attempt, leaseEpoch, event)
	if err != nil {
		return err
	}
	return d.Send(env)
}

func (d *Daemon) sendHello() error {
	hello := &runtimev1.Hello{
		DaemonId:          d.cfg.DaemonID,
		AuthToken:         d.cfg.AuthToken,
		RuntimeName:       d.cfg.RuntimeName,
		RuntimeType:       d.cfg.RuntimeType,
		MaxConcurrentRuns: d.cfg.MaxConcurrentRuns,
		Version:           d.cfg.Version,
		Labels:            d.cfg.Labels,
	}
	env, err := protocol.New(hello)
	if err != nil {
		return err
	}
	return d.Send(env)
}

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hb := &runtimev1.Heartbeat{RunningCount: d.running.Load()}
			env, err := protocol.New(hb)
			if err != nil {
				continue
			}
			_ = d.Send(env)
		}
	}
}

func (d *Daemon) recvLoop(ctx context.Context) {
	for {
		env, err := d.stream.Recv()
		if err != nil {
			d.setErr(err)
			return
		}

		if result := env.GetToolCallResult(); result != nil {
			if value, ok := d.pending.Load(result.GetInvocationId()); ok {
				select {
				case value.(chan *runtimev1.ToolCallResult) <- result:
				default:
				}
			}
			continue
		}

		if env.GetTaskAssign() != nil {
			select {
			case d.assignments <- env:
			default:
			}
		}

		if d.onMessage != nil {
			if err := d.onMessage(ctx, env, d); err != nil {
				d.setErr(err)
				return
			}
			continue
		}

		if env.GetTaskAssign() != nil {
			if err := d.handleTaskAssign(env); err != nil {
				d.setErr(err)
				return
			}
		}
	}
}

func (d *Daemon) handleTaskAssign(env *runtimev1.Envelope) error {
	assign := env.GetTaskAssign()
	reason := d.cfg.RejectReason
	if !d.cfg.Accept && reason == "" {
		reason = "rejected"
	}
	ack := &runtimev1.TaskAck{Accepted: d.cfg.Accept, Reason: reason}
	if d.cfg.AckDelay > 0 {
		time.Sleep(d.cfg.AckDelay)
	}
	ackEnv, err := protocol.NewResponse(env, ack)
	if err != nil {
		return err
	}
	if err := d.Send(ackEnv); err != nil {
		return err
	}
	if !d.cfg.Accept {
		return nil
	}

	d.running.Add(1)
	defer d.running.Add(-1)
	if d.cfg.AutoEcho {
		if err := d.SendEvent(assign.GetRunId(), env.GetAttempt(), env.GetLeaseEpoch(), runtimev1.EventType_RUN_STATUS, map[string]any{"status": "running"}); err != nil {
			return err
		}
		if err := d.SendEvent(assign.GetRunId(), env.GetAttempt(), env.GetLeaseEpoch(), runtimev1.EventType_MESSAGE_COMPLETED, map[string]any{"role": "assistant", "content": "echo"}); err != nil {
			return err
		}
		if err := d.SendEvent(assign.GetRunId(), env.GetAttempt(), env.GetLeaseEpoch(), runtimev1.EventType_RUN_STATUS, map[string]any{"status": "completed"}); err != nil {
			return err
		}
	}
	if d.cfg.DisconnectAfterAssignments > 0 {
		// Closing the stream simulates a daemon crash after handling the task.
		d.cancel()
	}
	return nil
}

func (d *Daemon) nextSeq(runID string) int64 {
	d.seqMu.Lock()
	defer d.seqMu.Unlock()
	d.seq[runID]++
	return d.seq[runID]
}

func (d *Daemon) setErr(err error) {
	select {
	case d.errCh <- err:
	default:
	}
}

func normalize(cfg Config) Config {
	if cfg.DaemonID == "" {
		cfg.DaemonID = "fake-daemon"
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = "test-token"
	}
	if cfg.RuntimeType == runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED {
		cfg.RuntimeType = runtimev1.RuntimeType_NORMAL
	}
	if cfg.RuntimeName == "" {
		cfg.RuntimeName = "fake1"
	}
	if cfg.MaxConcurrentRuns <= 0 {
		cfg.MaxConcurrentRuns = 4
	}
	if cfg.Version == "" {
		cfg.Version = "test"
	}
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{"pool": "test"}
	}
	return cfg
}
