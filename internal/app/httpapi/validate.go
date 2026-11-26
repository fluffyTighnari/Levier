// Levier — internal/app/httpapi
//
// App HTTP request validation.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxToolTimeoutMS = 240_000

// Validate checks a create-run request.
func (r CreateRunRequest) Validate() error {
	if strings.TrimSpace(r.RuntimeName) == "" {
		return fmt.Errorf("runtime_name is required")
	}
	switch r.Runtime {
	case "", RuntimeNormal, RuntimeSandbox, RuntimeLongterm:
	default:
		return fmt.Errorf("runtime must be normal, sandbox, or longterm")
	}
	// A missing provider selects the control-plane default provider. A missing
	// model selects that provider's default model.
	if r.Timeouts.RunMS < 0 || r.Timeouts.ToolMS < 0 {
		return fmt.Errorf("timeouts must not be negative")
	}
	if r.Timeouts.ToolMS > maxToolTimeoutMS {
		return fmt.Errorf("timeouts.tool_ms must be <= %d", maxToolTimeoutMS)
	}
	for i, message := range r.Input {
		if message.Role == "" || message.Content == "" {
			return fmt.Errorf("input[%d].role and content are required", i)
		}
	}
	if r.Runtime != RuntimeLongterm && r.SessionID != "" {
		return fmt.Errorf("session_id is only valid for longterm runs")
	}
	return nil
}

// Validate checks an App upsert request.
func (r AppUpsertRequest) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	seen := make(map[string]struct{}, len(r.Tools))
	for i, tool := range r.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tools[%d].name is required", i)
		}
		if _, ok := seen[tool.Name]; ok {
			return fmt.Errorf("tools contains duplicate name %q", tool.Name)
		}
		seen[tool.Name] = struct{}{}
		if len(tool.Schema) > 0 && !json.Valid(tool.Schema) {
			return fmt.Errorf("tools[%d].schema is not valid JSON", i)
		}
		if tool.TimeoutMS < 0 {
			return fmt.Errorf("tools[%d].timeout_ms must not be negative", i)
		}
		if tool.TimeoutMS > maxToolTimeoutMS {
			return fmt.Errorf("tools[%d].timeout_ms must be <= %d", i, maxToolTimeoutMS)
		}
	}
	return nil
}
