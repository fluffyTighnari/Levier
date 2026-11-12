// Middleware — internal/httpapi
//
// App HTTP/WS contract and golden JSON tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"encoding/json"
	"github.com/fluffyTighnari/middleware/internal/contract"
	"strings"
	"testing"
)

func TestCreateRunRequestGolden(t *testing.T) {
	req := CreateRunRequest{
		Runtime: RuntimeNormal,
		Strategy: StrategyRequest{
			Name:   "react",
			Params: map[string]any{"max_steps": 32},
		},
		Input: []ChatMessage{{Role: "user", Content: "hi"}},
		Model: ModelRequest{
			Provider: "openai",
			Model:    "gpt-test",
			Params:   map[string]string{"temperature": "0.7"},
		},
		Timeouts: TimeoutRequest{RunMS: 1800000, ToolMS: 60000},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"runtime":"normal","strategy":{"name":"react","params":{"max_steps":32}},"input":[{"role":"user","content":"hi"}],"model":{"provider":"openai","model":"gpt-test","params":{"temperature":"0.7"}},"timeouts":{"run_ms":1800000,"tool_ms":60000}}`
	if string(data) != want {
		t.Fatalf("CreateRunRequest JSON\n got: %s\nwant: %s", data, want)
	}
}

func TestCreateRunResponseGolden(t *testing.T) {
	resp := CreateRunResponse{
		RunID:   "run-1",
		Status:  RunStatusQueued,
		WSURL:   WSURL,
		WSToken: "tok",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"run_id":"run-1","status":"queued","ws_url":"/api/v1/ws","ws_token":"tok"}`
	if string(data) != want {
		t.Fatalf("CreateRunResponse JSON\n got: %s\nwant: %s", data, want)
	}
}

func TestRunEventViewGolden(t *testing.T) {
	event := RunEventView{
		RunID:     "run-1",
		Attempt:   1,
		Seq:       2,
		Source:    "daemon",
		EventType: "MESSAGE_COMPLETED",
		Data:      json.RawMessage(`{"ok":true}`),
		TSMillis:  123,
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"run_id":"run-1","attempt":1,"seq":2,"source":"daemon","event_type":"MESSAGE_COMPLETED","data":{"ok":true},"ts_ms":123}`
	if string(data) != want {
		t.Fatalf("RunEventView JSON\n got: %s\nwant: %s", data, want)
	}
}

func TestEventMessageGolden(t *testing.T) {
	msg := EventMessage{
		Type:      contract.WSMessageEvent,
		RunID:     "run-1",
		Attempt:   1,
		Seq:       2,
		Source:    "daemon",
		EventType: "RUN_STATUS",
		Data:      json.RawMessage(`{"status":"running"}`),
		TSMillis:  123,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"type":"event","run_id":"run-1","attempt":1,"seq":2,"source":"daemon","event_type":"RUN_STATUS","data":{"status":"running"},"ts_ms":123}`
	if string(data) != want {
		t.Fatalf("EventMessage JSON\n got: %s\nwant: %s", data, want)
	}
}

func TestParseClientMessages(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "subscribe",
			data: `{"type":"subscribe","run_id":"run-1","last_attempt":2,"last_seq":42}`,
			want: "subscribe",
		},
		{
			name: "local tool result",
			data: `{"type":"local_tool_result","correlation_id":"c-1","invocation_id":"i-1","ok":true,"result_json":"{}"}`,
			want: "local_tool_result",
		},
		{
			name: "hello",
			data: `{"type":"hello","protocol":1}`,
			want: "hello",
		},
		{
			name: "ping",
			data: `{"type":"ping"}`,
			want: "ping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseClientMessage([]byte(tt.data))
			if err != nil {
				t.Fatalf("ParseClientMessage() error = %v", err)
			}
			switch tt.want {
			case "subscribe":
				if _, ok := msg.(*SubscribeMessage); !ok {
					t.Fatalf("message type = %T", msg)
				}
			case "local_tool_result":
				if _, ok := msg.(*LocalToolResultMessage); !ok {
					t.Fatalf("message type = %T", msg)
				}
			case "hello":
				if _, ok := msg.(*HelloMessage); !ok {
					t.Fatalf("message type = %T", msg)
				}
			case "ping":
				if _, ok := msg.(*PingMessage); !ok {
					t.Fatalf("message type = %T", msg)
				}
			}
		})
	}
}

func TestParseClientMessageErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"unknown", `{"type":"nope"}`, "unsupported ws message"},
		{"bad protocol", `{"type":"hello","protocol":2}`, "unsupported ws protocol"},
		{"missing run", `{"type":"subscribe","last_attempt":0,"last_seq":0}`, "run_id"},
		{"bad tool result", `{"type":"local_tool_result","correlation_id":"c","invocation_id":"i","ok":false}`, "error is required"},
		{"bad json", `{`, "decode ws message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseClientMessage([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseClientMessage() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateCreateRunRequest(t *testing.T) {
	valid := CreateRunRequest{
		Runtime:  RuntimeNormal,
		Strategy: StrategyRequest{Name: "react"},
		Input:    []ChatMessage{{Role: "user", Content: "hi"}},
		Model:    ModelRequest{Provider: "openai", Model: "gpt-test"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*CreateRunRequest)
		want   string
	}{
		{"bad runtime", func(r *CreateRunRequest) { r.Runtime = "vm" }, "runtime"},
		{"missing strategy", func(r *CreateRunRequest) { r.Strategy.Name = "" }, "strategy.name"},
		{"missing model", func(r *CreateRunRequest) { r.Model.Provider = "" }, "model.provider"},
		{"negative timeout", func(r *CreateRunRequest) { r.Timeouts.RunMS = -1 }, "timeouts"},
		{"tool timeout too large", func(r *CreateRunRequest) { r.Timeouts.ToolMS = maxToolTimeoutMS + 1 }, "tool_ms"},
		{"session on normal", func(r *CreateRunRequest) { r.SessionID = "s-1" }, "session_id"},
		{"empty input role", func(r *CreateRunRequest) { r.Input[0].Role = "" }, "input[0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			req.Input = []ChatMessage{{Role: "user", Content: "hi"}}
			tt.mutate(&req)
			err := req.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateAppUpsertRequest(t *testing.T) {
	valid := AppUpsertRequest{
		Name: "app-1",
		Tools: []ToolDefinition{{
			Name:   "echo",
			Schema: json.RawMessage(`{"type":"object"}`),
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*AppUpsertRequest)
		want   string
	}{
		{"missing name", func(r *AppUpsertRequest) { r.Name = "" }, "name"},
		{"duplicate tool", func(r *AppUpsertRequest) {
			r.Tools = append(r.Tools, ToolDefinition{Name: "echo", Schema: json.RawMessage(`{}`)})
		}, "duplicate name"},
		{"bad schema", func(r *AppUpsertRequest) { r.Tools[0].Schema = json.RawMessage(`{`) }, "not valid JSON"},
		{"negative timeout", func(r *AppUpsertRequest) { r.Tools[0].TimeoutMS = -1 }, "timeout_ms"},
		{"tool timeout too large", func(r *AppUpsertRequest) { r.Tools[0].TimeoutMS = maxToolTimeoutMS + 1 }, "timeout_ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			req.Tools = []ToolDefinition{{Name: "echo", Schema: json.RawMessage(`{}`)}}
			tt.mutate(&req)
			err := req.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
