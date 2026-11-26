// Levier — control
//
// Split from control.go (P8R.3).
// Copyright 2026 The Levier Authors.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/contract/protocol"
	"github.com/fluffyTighnari/levier/internal/state/register"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

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
	case "completed", "failed", "cancelled", "timeout", "running", "waiting":
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
	if c.cfg.ModelProviders != nil {
		var profile register.ModelProfile
		var err error
		if assign.Model != nil {
			profile, err = c.cfg.ModelProviders.ResolveModelProfile(
				ctx,
				assign.Model.Provider,
				assign.Model.Model,
				assign.Model.Params,
			)
		} else {
			profile, err = c.cfg.ModelProviders.ResolveDefaultModelProfile(ctx, nil)
		}
		if err != nil && !errors.Is(err, register.ErrModelProviderNotFound) {
			return workflow.Permanent(fmt.Errorf("resolve model provider: %w", err))
		}
		if err == nil {
			if assign.Model == nil {
				assign.Model = &runtimev1.ModelProfile{}
			}
			assign.Model.Provider = profile.Provider
			assign.Model.Model = profile.Model
			assign.Model.BaseUrl = profile.BaseURL
			assign.Model.ApiKey = profile.APIKey
			assign.Model.Params = profile.Params
		}
	}

	resume, err := c.resumeCheckpoint(ctx, run)
	if err != nil {
		return fmt.Errorf("load run checkpoint: %w", err)
	}
	if resume != nil {
		assign.ResumeFrom = resume
	}

	placement := PlacementRequest{
		Runtime:     runtimeType(run.Runtime),
		RuntimeName: run.RuntimeName,
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
		if !c.placement.Reserve(daemonID, run.RunID, capacityLeaseTTL(run)) {
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
			c.transport.Release(daemonID, run.RunID)
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
				"runtime": run.Runtime, "tenant": run.TenantID,
			}, assigned.AssignedAt.Sub(assigned.CreatedAt).Seconds())
		}
		c.log.Info("control assigned run",
			observe.FieldRunID, run.RunID,
			observe.FieldTraceID, run.TraceID,
			observe.FieldAppID, run.AppID,
			observe.FieldTenantID, run.TenantID,
			observe.FieldAttempt, attempt,
			observe.FieldLeaseEpoch, leaseEpoch,
			observe.FieldDaemonID, daemonID,
			"runtime", run.Runtime,
			"runtime_name", run.RuntimeName,
		)

		envelope, err := protocol.NewRun(run.RunID, int32(attempt), leaseEpoch, assign)
		if err != nil {
			c.transport.Release(daemonID, run.RunID)
			return workflow.Permanent(fmt.Errorf("build task assign: %w", err))
		}
		if c.dispatcher != nil {
			ack, dispatchErr := c.dispatcher.Dispatch(ctx, daemonID, envelope, c.cfg.AckTimeout)
			if dispatchErr != nil {
				c.transport.Release(daemonID, run.RunID)
				if errors.Is(dispatchErr, connection.ErrAckTimeout) {
					lastErr = errAckTimeout
					if c.cfg.Scheduler == nil {
						return errAckTimeout
					}
					excluded[daemonID] = struct{}{}
					attempt++
					leaseEpoch++
					continue
				}
				return fmt.Errorf("dispatch task assign: %w", dispatchErr)
			}
			if ack.GetAccepted() {
				return nil
			}
			c.transport.Release(daemonID, run.RunID)
			excluded[daemonID] = struct{}{}
			lastErr = fmt.Errorf("daemon %q rejected task: %s", daemonID, ack.GetReason())
			if c.cfg.Scheduler == nil {
				return lastErr
			}
			attempt++
			leaseEpoch++
			continue
		}

		ackCh := make(chan *runtimev1.TaskAck, 1)
		c.pendingMu.Lock()
		c.pending[envelope.GetMsgId()] = ackCh
		c.pendingMu.Unlock()

		if err := c.transport.Send(daemonID, envelope); err != nil {
			c.removePending(envelope.GetMsgId())
			c.transport.Release(daemonID, run.RunID)
			return fmt.Errorf("send task assign: %w", err)
		}

		timer := time.NewTimer(c.cfg.AckTimeout)
		select {
		case <-ctx.Done():
			timer.Stop()
			c.removePending(envelope.GetMsgId())
			c.transport.Release(daemonID, run.RunID)
			return ctx.Err()
		case <-timer.C:
			c.removePending(envelope.GetMsgId())
			c.transport.Release(daemonID, run.RunID)
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
			c.transport.Release(daemonID, run.RunID)
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
	return c.placement.PickHealthy(placement.Runtime, placement.RuntimeName, placement.Labels)
}

func (c *Control) removePending(msgID string) {
	c.pendingMu.Lock()
	delete(c.pending, msgID)
	c.pendingMu.Unlock()
}

// capacityLeaseTTL keeps the DB reservation alive for the run deadline plus a
// small teardown grace period. A missing run timeout falls back to the same
// conservative hour used by the local workflow lease.
func capacityLeaseTTL(run runstore.Run) time.Duration {
	if run.RunTimeoutMS > 0 {
		return time.Duration(run.RunTimeoutMS)*time.Millisecond + time.Minute
	}
	return time.Hour
}

func buildTaskAssign(run runstore.Run, attempt int) (*runtimev1.TaskAssign, error) {
	params := map[string]any{}
	if len(run.Params) > 0 {
		if err := json.Unmarshal(run.Params, &params); err != nil {
			return nil, fmt.Errorf("decode run params: %w", err)
		}
	}
	runtimeParams, err := structpb.NewStruct(params)
	if err != nil {
		return nil, fmt.Errorf("build runtime params: %w", err)
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
	assign := &runtimev1.TaskAssign{
		RunId:  run.RunID,
		AppId:  run.AppID,
		Params: runtimeParams,
		Input:  messages,
		Timeouts: &runtimev1.TimeoutProfile{
			RunTimeoutMs:  runTimeout,
			ToolTimeoutMs: toolTimeout,
		},
		SessionId:       run.SessionID,
		TraceContext:    run.TraceID,
		ResumeValueJson: run.ResumeJSON,
	}
	if model.Provider != "" || model.Model != "" {
		assign.Model = &runtimev1.ModelProfile{
			Provider: model.Provider,
			Model:    model.Model,
			ApiKey:   model.APIKey,
			BaseUrl:  model.BaseURL,
			Params:   model.Params,
		}
	}
	return assign, nil
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
