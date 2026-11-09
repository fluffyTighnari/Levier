// Middleware — internal/gateway/executor
//
// builtin:http executor tests.
//
// Copyright 2026 The Middleware Authors.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/gateway"
	"github.com/fluffyTighnari/middleware/internal/register"
)

func callWith(config string) gateway.GlobalCall {
	return gateway.GlobalCall{
		RunID:          "run-1",
		AppID:          "app-1",
		InvocationID:   "inv-1",
		ToolName:       "fetch",
		ArgumentsJSON:  `{"q":"hello world"}`,
		ExecutorRef:    "builtin:http",
		ToolConfigJSON: config,
		Timeout:        time.Second,
	}
}

func TestHTTPExecuteRendersRequestAndEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "hello world" {
			t.Errorf("url = %s", r.URL.String())
		}
		if r.Header.Get("X-Token") != "secret" {
			t.Errorf("token header = %q", r.Header.Get("X-Token"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"query":"hello world"}` {
			t.Errorf("body = %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{
		"method": "POST",
		"url": %q,
		"headers": {"X-Token": "secret"},
		"body": "{\"query\":{{json .args.q}}}"
	}`, server.URL+"/search?q={{urlquery .args.q}}")
	result, err := NewHTTP().Execute(context.Background(), callWith(config))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %+v", result)
	}
	var envelope struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.ResultJSON), &envelope); err != nil {
		t.Fatalf("result JSON = %q: %v", result.ResultJSON, err)
	}
	if envelope.Status != http.StatusCreated || string(envelope.Body) != `{"ok":true}` {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestHTTPExecuteDefaultMethodAndTextBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte("plain text"))
	}))
	defer server.Close()

	result, err := NewHTTP().Execute(context.Background(), callWith(fmt.Sprintf(`{"url":%q}`, server.URL)))
	if err != nil || !result.OK {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	var envelope struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.ResultJSON), &envelope); err != nil {
		t.Fatalf("result JSON = %q: %v", result.ResultJSON, err)
	}
	if envelope.Status != http.StatusOK || envelope.Body != "plain text" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestHTTPExecuteStatusErrors(t *testing.T) {
	tests := []struct {
		status    int
		wantCode  string
		retryable bool
	}{
		{http.StatusNotFound, "HTTP_4XX", false},
		{http.StatusTooManyRequests, "HTTP_4XX", true},
		{http.StatusInternalServerError, "HTTP_5XX", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("upstream said no"))
			}))
			defer server.Close()

			result, err := NewHTTP().Execute(context.Background(), callWith(fmt.Sprintf(`{"url":%q}`, server.URL)))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.OK || result.Code != tt.wantCode || result.Retryable != tt.retryable {
				t.Fatalf("result = %+v", result)
			}
			if !strings.Contains(result.Message, "upstream said no") {
				t.Fatalf("message = %q", result.Message)
			}
			if attempts.Load() != 1 {
				t.Fatalf("attempts = %d", attempts.Load())
			}
		})
	}
}

func TestHTTPRetriesTransientFailure(t *testing.T) {
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
	result, err := NewHTTP().Execute(context.Background(), callWith(config))
	if err != nil || !result.OK {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}

func TestHTTPDoesNotRetry4xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	config := fmt.Sprintf(`{"url":%q,"retries":2,"retry_backoff_ms":1}`, server.URL)
	result, err := NewHTTP().Execute(context.Background(), callWith(config))
	if err != nil || result.OK || result.Retryable {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}

func TestHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := NewHTTP().Execute(ctx, callWith(fmt.Sprintf(`{"url":%q}`, server.URL)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHTTPResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{"url":%q,"max_response_bytes":16}`, server.URL)
	result, err := NewHTTP().Execute(context.Background(), callWith(config))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.OK || result.Code != "RESPONSE_TOO_LARGE" || result.Retryable {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPInvalidConfigAndArguments(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		arguments string
	}{
		{"empty config", "", `{}`},
		{"invalid json", `{`, `{}`},
		{"missing url", `{"method":"GET"}`, `{}`},
		{"unsupported method", `{"url":"http://example.com","method":"TRACE"}`, `{}`},
		{"invalid scheme", `{"url":"file:///etc/passwd"}`, `{}`},
		{"unknown field", `{"url":"http://example.com","unexpected":true}`, `{}`},
		{"retries too high", `{"url":"http://example.com","retries":4}`, `{}`},
		{"response limit too low", `{"url":"http://example.com","max_response_bytes":-1}`, `{}`},
		{"arguments not object", fmt.Sprintf(`{"url":%q}`, "http://example.com"), `[1,2]`},
		{"arguments invalid json", fmt.Sprintf(`{"url":%q}`, "http://example.com"), `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := callWith(tt.config)
			call.ArgumentsJSON = tt.arguments
			result, err := NewHTTP().Execute(context.Background(), call)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.OK || result.Code != "TOOL_EXEC_ERROR" || result.Retryable {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestHTTPTemplateMissingKey(t *testing.T) {
	result, err := NewHTTP().Execute(context.Background(), callWith(`{"url":"http://example.com/{{.args.missing}}"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.OK || result.Code != "TOOL_EXEC_ERROR" {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseHTTPConfigDefaults(t *testing.T) {
	cfg, err := ParseHTTPConfig(`{"url":"https://example.com"}`)
	if err != nil {
		t.Fatalf("ParseHTTPConfig() error = %v", err)
	}
	if cfg.Method != http.MethodGet || cfg.MaxResponseBytes != defaultMaxResponseBytes ||
		cfg.Retries != 0 || cfg.RetryBackoffMS != defaultRetryBackoffMS {
		t.Fatalf("config = %+v", cfg)
	}
}

type staticResolver struct {
	tool register.ToolMaterial
}

func (s staticResolver) ResolveToolDecision(context.Context, string, string) (register.ToolMaterial, register.Resolution, error) {
	return s.tool, register.ResolutionFound, nil
}

func TestHTTPExecutorThroughInvoker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"echo":true}`))
	}))
	defer server.Close()

	config := fmt.Sprintf(`{"url":%q}`, server.URL)
	invoker := gateway.NewInvoker(
		staticResolver{tool: register.ToolMaterial{
			Name:        "fetch",
			Scope:       "global",
			ExecutorRef: "builtin:http",
			ConfigJSON:  config,
			TimeoutMS:   1000,
		}},
		nil,
		gateway.NewExecutorRegistry(map[string]gateway.GlobalExecutor{
			"builtin:http": NewHTTP(),
		}),
	)
	result := invoker.Invoke(context.Background(), "run-1", "app-1", &runtimev1.ToolCallRequest{
		InvocationId:  "inv-http-1",
		ToolName:      "fetch",
		ArgumentsJson: `{}`,
		TimeoutMs:     1000,
	})
	if !result.GetOk() {
		t.Fatalf("result = %+v", result)
	}
	var envelope struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.GetResultJson()), &envelope); err != nil {
		t.Fatalf("result JSON = %q: %v", result.GetResultJson(), err)
	}
	if envelope.Status != http.StatusOK || string(envelope.Body) != `{"echo":true}` {
		t.Fatalf("envelope = %+v", envelope)
	}
}
