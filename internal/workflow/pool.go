// Middleware — internal/workflow
//
// RunTaskQueue worker pool.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"fmt"
	"sync"
)

// Pool manages a fixed set of workers.
type Pool struct {
	workers    []*Worker
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
	workers := make([]*Worker, workerCount)
	for i := range workers {
		workerCfg := cfg
		workerCfg.Owner = fmt.Sprintf("%s-worker-%d", cfg.Owner, i+1)
		workers[i] = NewWorker(store, handler, workerCfg)
	}
	return &Pool{workers: workers}
}

// Size returns the number of workers.
func (p *Pool) Size() int {
	return len(p.workers)
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

	for _, worker := range p.workers {
		worker.Stop()
	}
}
