// Middleware — internal/workflow
//
// RunTaskQueue worker loop.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// WorkerConfig controls one worker's claim and retry behavior.
type WorkerConfig struct {
	Owner             string
	LeaseTTL          time.Duration
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	RetryBase         time.Duration
	RetryMax          time.Duration
	ShutdownTimeout   time.Duration
}

// DefaultWorkerConfig returns a production-oriented worker config.
func DefaultWorkerConfig(owner string) WorkerConfig {
	if owner == "" {
		owner = "worker"
	}
	return WorkerConfig{
		Owner:             owner,
		LeaseTTL:          30 * time.Second,
		PollInterval:      200 * time.Millisecond,
		HeartbeatInterval: 10 * time.Second,
		RetryBase:         2 * time.Second,
		RetryMax:          5 * time.Minute,
		ShutdownTimeout:   5 * time.Second,
	}
}

// Worker claims and handles one task at a time.
type Worker struct {
	store      Store
	handler    Handler
	cfg        WorkerConfig
	deadLetter DeadLetterHandler
	onError    func(error)

	mu        sync.Mutex
	started   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	runCancel context.CancelFunc
	stopOnce  sync.Once
}

// NewWorker creates a worker. Zero-valued config fields use defaults.
func NewWorker(store Store, handler Handler, cfg WorkerConfig) *Worker {
	if cfg.Owner == "" {
		cfg.Owner = "worker"
	}
	def := DefaultWorkerConfig(cfg.Owner)
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = def.LeaseTTL
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = def.PollInterval
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = cfg.LeaseTTL / 3
	}
	if cfg.HeartbeatInterval < 10*time.Millisecond {
		cfg.HeartbeatInterval = 10 * time.Millisecond
	}
	if cfg.RetryBase <= 0 {
		cfg.RetryBase = def.RetryBase
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = def.RetryMax
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = def.ShutdownTimeout
	}
	return &Worker{
		store:   store,
		handler: handler,
		cfg:     cfg,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// SetDeadLetterHandler installs the DEAD-task callback.
func (w *Worker) SetDeadLetterHandler(handler DeadLetterHandler) {
	w.deadLetter = handler
}

// SetErrorHandler installs a loop/store error callback.
func (w *Worker) SetErrorHandler(handler func(error)) {
	w.onError = handler
}

// Start launches the claim loop. It is not restartable.
func (w *Worker) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	ctx, cancel := context.WithCancel(context.Background())
	w.runCancel = cancel
	w.mu.Unlock()

	go func() {
		defer close(w.doneCh)
		w.run(ctx)
	}()
}

// Stop stops claiming, waits for the current handler, and cancels it after
// ShutdownTimeout.
func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	started := w.started
	w.mu.Unlock()
	if !started {
		return
	}

	w.stopOnce.Do(func() {
		close(w.stopCh)
	})

	select {
	case <-w.doneCh:
		return
	case <-time.After(w.cfg.ShutdownTimeout):
		if w.runCancel != nil {
			w.runCancel()
		}
		<-w.doneCh
	}
}

// ProcessOne claims and handles at most one task. It returns false when no
// task is ready.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	task, lease, err := w.store.Claim(ctx, w.cfg.Owner, w.cfg.LeaseTTL)
	if errors.Is(err, ErrNoTask) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go w.heartbeat(handlerCtx, lease, cancel, stopHeartbeat, heartbeatDone)

	handlerErr := w.safeHandle(handlerCtx, task, lease)
	close(stopHeartbeat)
	<-heartbeatDone

	if err := w.finish(ctx, task, lease, handlerErr); err != nil {
		return true, err
	}
	return true, nil
}

func (w *Worker) run(ctx context.Context) {
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		processed, err := w.ProcessOne(ctx)
		if err != nil {
			w.reportError(err)
			if !w.wait(ctx, w.cfg.PollInterval) {
				return
			}
			continue
		}
		if !processed {
			if !w.wait(ctx, w.cfg.PollInterval) {
				return
			}
		}
	}
}

func (w *Worker) safeHandle(ctx context.Context, task Task, lease Lease) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panic: %v", recovered)
		}
	}()
	return w.handler.Handle(ctx, task, lease)
}

func (w *Worker) finish(ctx context.Context, task Task, lease Lease, handlerErr error) error {
	if handlerErr == nil {
		if err := w.store.Ack(ctx, task, lease); err != nil && !isTerminalLeaseError(err) {
			return err
		}
		return nil
	}

	if IsPermanent(handlerErr) {
		if err := w.store.Fail(ctx, task, lease, handlerErr); err != nil {
			if !isTerminalLeaseError(err) {
				return err
			}
			return nil
		}
		w.notifyDead(ctx, task, handlerErr)
		return nil
	}

	delay, explicit := RetryAfterDuration(handlerErr)
	if !explicit {
		delay = Backoff(task.Attempt, w.cfg.RetryBase, w.cfg.RetryMax)
	}
	retryAt := time.Now().UTC().Add(delay)
	if err := w.store.Nack(ctx, task, lease, retryAt, handlerErr); err != nil {
		if !isTerminalLeaseError(err) {
			return err
		}
		return nil
	}
	if task.Attempt >= task.MaxAttempts {
		w.notifyDead(ctx, task, handlerErr)
	}
	return nil
}

func (w *Worker) heartbeat(ctx context.Context, lease Lease, cancel context.CancelFunc, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	interval := w.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = w.cfg.LeaseTTL / 3
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.store.Extend(ctx, lease, w.cfg.LeaseTTL); err != nil {
				cancel()
				return
			}
		}
	}
}

func (w *Worker) notifyDead(ctx context.Context, task Task, cause error) {
	if w.deadLetter == nil {
		return
	}
	if err := w.deadLetter.HandleDead(ctx, task, cause); err != nil {
		w.reportError(err)
	}
}

func (w *Worker) reportError(err error) {
	if err != nil && w.onError != nil {
		w.onError(err)
	}
}

func (w *Worker) wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-w.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func isTerminalLeaseError(err error) bool {
	return errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrTaskTerminal) || errors.Is(err, ErrTaskNotFound)
}
