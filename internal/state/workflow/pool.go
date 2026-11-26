// Levier — internal/state/workflow
//
// RunTaskQueue worker pool.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"fmt"
	"sync"
)

// Pool manages a fixed set of workers.
type Pool struct {
	store      Store
	workers    []*Worker
	metrics    *Metrics
	deadLetter DeadLetterHandler
	onError    func(error)

	mu      sync.Mutex
	started bool
}

// NewPool creates workerCount workers sharing store/handler.
func NewPool(store Store, handler Handler, workerCount int, cfg WorkerConfig) *Pool {
	if workerCount < 1 {
		workerCount = 1
	}
	if cfg.Owner == "" {
		cfg.Owner = "pool"
	}
	metrics := NewMetrics()
	workers := make([]*Worker, workerCount)
	for i := range workers {
		workerCfg := cfg
		workerCfg.Owner = fmt.Sprintf("%s-worker-%d", cfg.Owner, i+1)
		workers[i] = NewWorker(store, handler, workerCfg)
		workers[i].SetMetrics(metrics)
	}
	return &Pool{store: store, workers: workers, metrics: metrics}
}

// Size returns the number of workers.
func (p *Pool) Size() int {
	return len(p.workers)
}

// SetMetrics replaces the pool metrics registry.
func (p *Pool) SetMetrics(metrics *Metrics) {
	if metrics == nil {
		return
	}
	p.metrics = metrics
	for _, worker := range p.workers {
		worker.SetMetrics(metrics)
	}
}

// Metrics returns the pool metrics registry.
func (p *Pool) Metrics() *Metrics {
	return p.metrics
}

// Stats returns the current queue stats from the store.
func (p *Pool) Stats(ctx context.Context) (Stats, error) {
	return p.store.Stats(ctx)
}

// SetDeadLetterHandler installs the DEAD callback on every worker.
func (p *Pool) SetDeadLetterHandler(handler DeadLetterHandler) {
	p.deadLetter = handler
	for _, worker := range p.workers {
		worker.SetDeadLetterHandler(handler)
	}
}

// SetErrorHandler installs the loop/store error callback on every worker.
func (p *Pool) SetErrorHandler(handler func(error)) {
	p.onError = handler
	for _, worker := range p.workers {
		worker.SetErrorHandler(handler)
	}
}

// Start starts every worker.
func (p *Pool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	for _, worker := range p.workers {
		worker.Start()
	}
}

// Stop stops every worker and waits for in-flight handlers.
func (p *Pool) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.started = false
	p.mu.Unlock()

	// Stop workers concurrently so the total shutdown time is bounded by the
	// slowest worker, not by workers multiplied by their individual timeout.
	workers := make([]*Worker, len(p.workers))
	copy(workers, p.workers)
	done := make(chan struct{}, len(workers))
	for _, worker := range workers {
		go func(worker *Worker) {
			worker.Stop()
			done <- struct{}{}
		}(worker)
	}
	for range workers {
		<-done
	}
}
