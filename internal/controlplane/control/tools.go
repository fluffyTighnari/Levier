// Levier — control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"fmt"
	"github.com/fluffyTighnari/levier/internal/platform/errs"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
)

// HandleToolCall executes one daemon ToolCallRequest through the gateway.
func (c *Control) HandleToolCall(daemonID string, env *runtimev1.Envelope) error {
	if c.cfg.Tools == nil || env == nil || env.GetToolCallRequest() == nil {
		return nil
	}
	ctx := context.Background()
	if c.ctx != nil {
		ctx = c.ctx
	}
	run, err := c.runs.GetRun(ctx, env.GetRunId())
	if err != nil {
		return fmt.Errorf("get run for tool call: %w", err)
	}
	ctx = observe.WithTenant(ctx, run.TenantID)
	result := c.cfg.Tools.Invoke(ctx, env.GetRunId(), run.AppID, env.GetToolCallRequest())
	response, err := protocol.NewResponse(env, result)
	if err != nil {
		return fmt.Errorf("build tool call result: %w", err)
	}
	if err := c.transport.Send(daemonID, response); err != nil {
		return fmt.Errorf("send tool call result: %w", err)
	}
	return nil
}

// DispatchToolCall runs a tool call without blocking the daemon read loop.
//
// The read loop must keep processing heartbeats and events; a slow local tool
// would otherwise make a healthy daemon look offline after 30s.
func (c *Control) DispatchToolCall(daemonID string, env *runtimev1.Envelope) {
	if c == nil || env == nil || env.GetToolCallRequest() == nil {
		return
	}
	select {
	case c.toolSlots <- struct{}{}:
	default:
		c.rejectToolCall(daemonID, env, string(errs.CodeToolBusy), "too many concurrent tool calls", true)
		return
	}

	c.toolMu.Lock()
	if c.toolClosed {
		c.toolMu.Unlock()
		<-c.toolSlots
		return
	}
	c.toolWG.Add(1)
	c.toolMu.Unlock()

	go func() {
		defer c.toolWG.Done()
		defer func() { <-c.toolSlots }()
		if err := c.HandleToolCall(daemonID, env); err != nil {
			c.log.Warn("control tool call failed",
				observe.FieldDaemonID, daemonID,
				observe.FieldRunID, env.GetRunId(),
				observe.FieldInvocationID, env.GetToolCallRequest().GetInvocationId(),
				"err", err,
			)
		}
	}()
}

func (c *Control) rejectToolCall(daemonID string, env *runtimev1.Envelope, code, message string, retryable bool) {
	request := env.GetToolCallRequest()
	result := &runtimev1.ToolCallResult{
		InvocationId: request.GetInvocationId(),
		Ok:           false,
		Error: &runtimev1.ErrorInfo{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
	response, err := protocol.NewResponse(env, result)
	if err != nil {
		c.log.Warn("control build busy tool response failed",
			observe.FieldDaemonID, daemonID,
			observe.FieldRunID, env.GetRunId(),
			"err", err,
		)
		return
	}
	if err := c.transport.Send(daemonID, response); err != nil {
		c.log.Warn("control send busy tool response failed",
			observe.FieldDaemonID, daemonID,
			observe.FieldRunID, env.GetRunId(),
			"err", err,
		)
	}
}
