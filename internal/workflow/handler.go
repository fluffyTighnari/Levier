// Levier — internal/workflow
//
// Task handler contract and retry classification.
//
// Copyright 2026 The Levier Authors.
package workflow

import "context"

// Handler processes one claimed task.
type Handler interface {
	Handle(ctx context.Context, task Task, lease Lease) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, Task, Lease) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, task Task, lease Lease) error {
	return f(ctx, task, lease)
}
