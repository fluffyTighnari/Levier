// Middleware — internal/gateway/executor
//
// builtin:webhook executor tests.
//
// Copyright 2026 The Middleware Authors.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/gateway"
	"github.com/fluffyTighnari/middleware/internal/register"
)

func TestWebhookExecutePayloadAndResponseMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/hook" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Event") != "fetch" {
			t.Errorf("header = %q", r.Header.Get("X-Event"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"event":"fetch","invocation_id":"inv-1","data":{"q":"hello"}}` {
			t.Errorf("payload = %s", body)
		}
		_, _ = w.Write([]byte(`{"id":"wh-1","ok":true,"items":[{"name":"a"}]}`))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{
		"url": %q,
		"headers": {"X-Event": "{{.tool_name}}"},
		"payload": "{\"event\":\"{{.tool_name}}\",\"invocation_id\":\"{{.invocation_id}}\",\"data\":{{json .args}}}",
		"response_map": {
			"id": "body.id",
			"ok": "body.ok",
			"status": "status",
			"first": "body.items.0.name"
		}
	}`, server.URL+"/hook")
	call := callWith(config)
	call.ArgumentsJSON = `{"q":"hello"}`
	result, err := NewWebhook().Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %+v", result)
	}
	var mapped map[string]any
	if err := json.Unmarshal([]byte(result.ResultJSON), &mapped); err != nil {
		t.Fatalf("result JSON = %q: %v", result.ResultJSON, err)
	}
	if mapped["id"] != "wh-1" || mapped["ok"] != true || mapped["status"] != float64(200) || mapped["first"] != "a" {
		t.Fatalf("mapped = %+v", mapped)
	}
}

func TestWebhookDefaultPayloadAndEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"q":"hello world"}` {
			t.Errorf("payload = %s", body)
		}
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	result, err := NewWebhook().Execute(context.Background(), callWith(fmt.Sprintf(`{"url":%q}`, server.URL)))
	if err != nil || !result.OK {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	var envelope struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.ResultJSON), &envelope); err != nil {
		t.Fatalf("result JSON = %q: %v", result.ResultJSON, err)
	}
	if envelope.Status != http.StatusOK || string(envelope.Body) != `{"accepted":true}` {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestWebhookResponseMappingErrors(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		mapping    string
		wantOK     bool
		wantString string
	}{
		{"missing path", `{"ok":true}`, `{"value":"body.missing"}`, false, ""},
		{"non json with mapping", `plain`, `{"value":"body.ok"}`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			config := fmt.Sprintf(`{"url":%q,"response_map":%s}`, server.URL, tt.mapping)
			result, err := NewWebhook().Execute(context.Background(), callWith(config))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.OK || result.Code != "TOOL_EXEC_ERROR" || result.Retryable {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestWebhookStatusAndRetryBoundaries(t *testing.T) {
	t.Run("4xx is not retried", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		config := fmt.Sprintf(`{"url":%q,"retries":2,"retry_backoff_ms":1}`, server.URL)
		result, err := NewWebhook().Execute(context.Background(), callWith(config))
		if err != nil || result.OK || result.Code != "HTTP_4XX" || result.Retryable {
			t.Fatalf("Execute() = %+v, %v", result, err)
		}
		if attempts.Load() != 1 {
			t.Fatalf("attempts = %d", attempts.Load())
		}
	})

	t.Run("5xx is retried", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if attempts.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()

		config := fmt.Sprintf(`{"url":%q,"retries":1,"retry_backoff_ms":1}`, server.URL)
		result, err := NewWebhook().Execute(context.Background(), callWith(config))
		if err != nil || !result.OK {
			t.Fatalf("Execute() = %+v, %v", result, err)
		}
		if attempts.Load() != 2 {
			t.Fatalf("attempts = %d", attempts.Load())
		}
	})
}

func TestWebhookResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{"url":%q,"max_response_bytes":8}`, server.URL)
	result, err := NewWebhook().Execute(context.Background(), callWith(config))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.OK || result.Code != "RESPONSE_TOO_LARGE" {
		t.Fatalf("result = %+v", result)
	}
}

func TestWebhookInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{"empty", ""},
		{"missing url", `{}`},
		{"templated url", `{"url":"http://example.com/{{.args.path}}"}`},
		{"unsupported scheme", `{"url":"ftp://example.com"}`},
		{"method is fixed", `{"url":"http://example.com","method":"GET"}`},
		{"invalid header", `{"url":"http://example.com","headers":{"bad header":"x"}}`},
		{"bad response root", `{"url":"http://example.com","response_map":{"x":"payload.id"}}`},
		{"empty response path", `{"url":"http://example.com","response_map":{"x":""}}`},
		{"retries too high", `{"url":"http://example.com","retries":9}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewWebhook().Execute(context.Background(), callWith(tt.config))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.OK || result.Code != "TOOL_EXEC_ERROR" || result.Retryable {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestWebhookTemplateMissingKey(t *testing.T) {
	result, err := NewWebhook().Execute(context.Background(), callWith(`{
		"url":"http://example.com",
		"payload":"{{.args.missing}}"
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.OK || result.Code != "TOOL_EXEC_ERROR" {
		t.Fatalf("result = %+v", result)
	}
}

func TestWebhookThroughInvoker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"wh-1"}`))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{"url":%q,"response_map":{"id":"body.id"}}`, server.URL)
	invoker := gateway.NewInvoker(
		staticResolver{tool: register.ToolMaterial{
			Name:        "notify",
			Scope:       "global",
			ExecutorRef: "builtin:webhook",
			ConfigJSON:  config,
			TimeoutMS:   1000,
		}},
		nil,
		gateway.NewExecutorRegistry(map[string]gateway.GlobalExecutor{
			"builtin:webhook": NewWebhook(),
		}),
	)
	result := invoker.Invoke(context.Background(), "run-1", "app-1", &runtimev1.ToolCallRequest{
		InvocationId:  "inv-webhook-1",
		ToolName:      "notify",
		ArgumentsJson: `{"message":"hi"}`,
		TimeoutMs:     1000,
	})
	if !result.GetOk() {
		t.Fatalf("result = %+v", result)
	}
	var mapped map[string]any
	if err := json.Unmarshal([]byte(result.GetResultJson()), &mapped); err != nil {
		t.Fatalf("result JSON = %q: %v", result.GetResultJson(), err)
	}
	if mapped["id"] != "wh-1" {
		t.Fatalf("mapped = %+v", mapped)
	}
}
