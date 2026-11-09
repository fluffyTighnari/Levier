// Middleware — internal/gateway/executor
//
// builtin:http global tool executor: request templating, bounded retries,
// and response size limits.
//
// Copyright 2026 The Middleware Authors.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/fluffyTighnari/middleware/internal/gateway"
)

const (
	defaultMaxResponseBytes = 1 << 20
	maxResponseBytesLimit   = 8 << 20
	defaultRetryBackoffMS   = 100
	maxRetries              = 3
	errorSnippetBytes       = 512
)

var errResponseTooLarge = errors.New("response exceeds configured limit")

// HTTPConfig is the tool-specific configuration for builtin:http.
type HTTPConfig struct {
	Method           string            `json:"method"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Body             string            `json:"body"`
	MaxResponseBytes int64             `json:"max_response_bytes"`
	Retries          int               `json:"retries"`
	RetryBackoffMS   int               `json:"retry_backoff_ms"`
}

// HTTP executes builtin:http tools.
type HTTP struct {
	Client *http.Client
}

// NewHTTP returns a builtin:http executor.
func NewHTTP() *HTTP {
	return &HTTP{Client: &http.Client{}}
}

// ParseHTTPConfig validates and defaults one builtin:http config document.
func ParseHTTPConfig(raw string) (HTTPConfig, error) {
	cfg := HTTPConfig{
		Method:           http.MethodGet,
		MaxResponseBytes: defaultMaxResponseBytes,
		Retries:          0,
		RetryBackoffMS:   defaultRetryBackoffMS,
	}
	if strings.TrimSpace(raw) == "" {
		return HTTPConfig{}, fmt.Errorf("builtin:http requires config_json")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return HTTPConfig{}, fmt.Errorf("parse builtin:http config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return HTTPConfig{}, fmt.Errorf("parse builtin:http config: unexpected trailing data")
	}

	cfg.Method = strings.ToUpper(strings.TrimSpace(cfg.Method))
	if cfg.Method == "" {
		cfg.Method = http.MethodGet
	}
	switch cfg.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return HTTPConfig{}, fmt.Errorf("builtin:http: unsupported method %q", cfg.Method)
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return HTTPConfig{}, fmt.Errorf("builtin:http: url is required")
	}
	if !strings.Contains(cfg.URL, "{{") {
		if err := validateHTTPURL(cfg.URL); err != nil {
			return HTTPConfig{}, fmt.Errorf("builtin:http: %w", err)
		}
	}
	for name, value := range cfg.Headers {
		if textproto.CanonicalMIMEHeaderKey(name) == "" {
			return HTTPConfig{}, fmt.Errorf("builtin:http: invalid header name %q", name)
		}
		cfg.Headers[name] = value
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = defaultMaxResponseBytes
	}
	if cfg.MaxResponseBytes < 1 || cfg.MaxResponseBytes > maxResponseBytesLimit {
		return HTTPConfig{}, fmt.Errorf("builtin:http: max_response_bytes must be between 1 and %d", maxResponseBytesLimit)
	}
	if cfg.Retries < 0 || cfg.Retries > maxRetries {
		return HTTPConfig{}, fmt.Errorf("builtin:http: retries must be between 0 and %d", maxRetries)
	}
	if cfg.RetryBackoffMS < 0 || cfg.RetryBackoffMS > 5000 {
		return HTTPConfig{}, fmt.Errorf("builtin:http: retry_backoff_ms must be between 0 and 5000")
	}
	return cfg, nil
}

// Execute runs one builtin:http request and returns a JSON envelope.
func (h *HTTP) Execute(ctx context.Context, call gateway.GlobalCall) (gateway.GlobalResult, error) {
	cfg, err := ParseHTTPConfig(call.ToolConfigJSON)
	if err != nil {
		return failure("TOOL_EXEC_ERROR", err.Error(), false), nil
	}
	args, err := parseArguments(call.ArgumentsJSON)
	if err != nil {
		return failure("TOOL_EXEC_ERROR", err.Error(), false), nil
	}
	data := invocationData(call, args)

	attempts := cfg.Retries + 1
	var last gateway.GlobalResult
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, retryDelay(cfg, attempt)); err != nil {
				return gateway.GlobalResult{}, err
			}
		}
		result, retry, err := h.do(ctx, cfg, data)
		if err != nil {
			if ctx.Err() != nil {
				return gateway.GlobalResult{}, ctx.Err()
			}
			last = failure("TOOL_EXEC_ERROR", err.Error(), true)
			if attempt == attempts-1 {
				return last, nil
			}
			continue
		}
		if result.OK || !retry || attempt == attempts-1 {
			return result, nil
		}
		last = result
	}
	return last, nil
}

func (h *HTTP) do(ctx context.Context, cfg HTTPConfig, data map[string]any) (gateway.GlobalResult, bool, error) {
	renderedURL, err := renderTemplate("url", cfg.URL, data)
	if err != nil {
		return failure("TOOL_EXEC_ERROR", err.Error(), false), false, nil
	}
	if err := validateHTTPURL(renderedURL); err != nil {
		return failure("TOOL_EXEC_ERROR", err.Error(), false), false, nil
	}
	body, err := renderTemplate("body", cfg.Body, data)
	if err != nil {
		return failure("TOOL_EXEC_ERROR", err.Error(), false), false, nil
	}
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, cfg.Method, renderedURL, reader)
	if err != nil {
		return failure("TOOL_EXEC_ERROR", "build request: "+err.Error(), false), false, nil
	}
	for name, value := range cfg.Headers {
		rendered, err := renderTemplate("header "+name, value, data)
		if err != nil {
			return failure("TOOL_EXEC_ERROR", err.Error(), false), false, nil
		}
		if rendered == "" {
			continue
		}
		request.Header.Set(name, rendered)
	}
	if body != "" && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}

	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return gateway.GlobalResult{}, false, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		payload, err := readLimited(response.Body, cfg.MaxResponseBytes)
		if errors.Is(err, errResponseTooLarge) {
			return failure("RESPONSE_TOO_LARGE",
				fmt.Sprintf("response exceeds %d bytes", cfg.MaxResponseBytes), false), false, nil
		}
		if err != nil {
			return gateway.GlobalResult{}, false, err
		}
		encoded, err := encodeBody(response.StatusCode, payload)
		if err != nil {
			return failure("TOOL_EXEC_ERROR", "encode response: "+err.Error(), false), false, nil
		}
		return gateway.GlobalResult{OK: true, ResultJSON: encoded}, false, nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(response.Body, errorSnippetBytes))
	code := "HTTP_4XX"
	if response.StatusCode >= 500 {
		code = "HTTP_5XX"
	}
	message := fmt.Sprintf("http status %d", response.StatusCode)
	if text := strings.TrimSpace(string(snippet)); text != "" {
		message += ": " + text
	}
	retry := response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests
	return failure(code, message, retry), retry, nil
}

func parseArguments(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("arguments_json must be a JSON object: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func invocationData(call gateway.GlobalCall, args map[string]any) map[string]any {
	return map[string]any{
		"args":          args,
		"args_json":     call.ArgumentsJSON,
		"run_id":        call.RunID,
		"app_id":        call.AppID,
		"invocation_id": call.InvocationID,
		"tool_name":     call.ToolName,
	}
}

func renderTemplate(name, text string, data map[string]any) (string, error) {
	if text == "" {
		return "", nil
	}
	parsed, err := template.New(name).
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"json": func(value any) (string, error) {
				encoded, err := json.Marshal(value)
				return string(encoded), err
			},
			"urlquery": url.QueryEscape,
		}).
		Parse(text)
	if err != nil {
		return "", fmt.Errorf("render %s template: %w", name, err)
	}
	var buffer bytes.Buffer
	if err := parsed.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("render %s template: %w", name, err)
	}
	return buffer.String(), nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("url host is required")
	}
	return nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errResponseTooLarge
	}
	return data, nil
}

func encodeBody(status int, payload []byte) (string, error) {
	body := any(string(payload))
	if json.Valid(payload) {
		body = json.RawMessage(payload)
	}
	encoded, err := json.Marshal(map[string]any{
		"status": status,
		"body":   body,
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func retryDelay(cfg HTTPConfig, attempt int) time.Duration {
	base := time.Duration(cfg.RetryBackoffMS) * time.Millisecond
	if base <= 0 {
		base = defaultRetryBackoffMS * time.Millisecond
	}
	return base * time.Duration(attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func failure(code, message string, retryable bool) gateway.GlobalResult {
	return gateway.GlobalResult{
		OK:        false,
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}
