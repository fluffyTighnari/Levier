// Middleware — internal/httpapi
//
// App HTTP request validation.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"encoding/json"
	"fmt"
)

// Validate checks a create-run request.
func (r CreateRunRequest) Validate() error {
	switch r.Runtime {
	case RuntimeNormal, RuntimeSandbox, RuntimeLongterm:
	default:
		return fmt.Errorf("runtime must be normal, sandbox, or longterm")
	}
	if r.Strategy.Name == "" {
		return fmt.Errorf("strategy.name is required")
	}
	if r.Model.Provider == "" || r.Model.Model == "" {
		return fmt.Errorf("model.provider and model are required")
	}
	if r.Timeouts.RunMS < 0 || r.Timeouts.ToolMS < 0 {
		return fmt.Errorf("timeouts must not be negative")
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
	}
	return nil
}
