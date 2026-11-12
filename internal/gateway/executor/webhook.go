// Middleware — internal/gateway/executor
//
// builtin:webhook global tool executor: pre-registered URL, templated payload,
// and response field mapping.
//
// Copyright 2026 The Middleware Authors.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fluffyTighnari/middleware/internal/gateway"
)

// WebhookConfig is the tool-specific configuration for builtin:webhook.
type WebhookConfig struct {
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Payload          string            `json:"payload"`
	ResponseMap      map[string]string `json:"response_map"`
	MaxResponseBytes int64             `json:"max_response_bytes"`
	Retries          int               `json:"retries"`
	RetryBackoffMS   int               `json:"retry_backoff_ms"`
}

// Webhook executes builtin:webhook tools with a fixed POST URL.
type Webhook struct {
	Client *http.Client
}

// NewWebhook returns a builtin:webhook executor.
func NewWebhook() *Webhook {
	return &Webhook{Client: &http.Client{}}
}

// ParseWebhookConfig validates and defaults one builtin:webhook config document.
func ParseWebhookConfig(raw string) (WebhookConfig, error) {
	cfg := WebhookConfig{
		MaxResponseBytes: defaultMaxResponseBytes,
		Retries:          0,
		RetryBackoffMS:   defaultRetryBackoffMS,
	}
	if strings.TrimSpace(raw) == "" {
		return WebhookConfig{}, fmt.Errorf("builtin:webhook requires config_json")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return WebhookConfig{}, fmt.Errorf("parse builtin:webhook config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return WebhookConfig{}, fmt.Errorf("parse builtin:webhook config: unexpected trailing data")
	}

	if err := validateStaticURL(cfg.URL, "builtin:webhook"); err != nil {
		return WebhookConfig{}, err
	}
	if err := validateHeaders(cfg.Headers, "builtin:webhook"); err != nil {
		return WebhookConfig{}, err
	}
	if err := validateLimits(cfg.MaxResponseBytes, cfg.Retries, cfg.RetryBackoffMS, "builtin:webhook"); err != nil {
		return WebhookConfig{}, err
	}
	if err := validateResponseMap(cfg.ResponseMap); err != nil {
		return WebhookConfig{}, err
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = defaultMaxResponseBytes
	}
	return cfg, nil
}

// Execute posts a payload to the pre-registered webhook URL.
func (w *Webhook) Execute(ctx context.Context, call gateway.GlobalCall) (gateway.GlobalResult, error) {
	cfg, err := ParseWebhookConfig(call.ToolConfigJSON)
	if err != nil {
		return failure(string(errs.CodeToolExecError), err.Error(), false), nil
	}
	args, err := parseArguments(call.ArgumentsJSON)
	if err != nil {
		return failure(string(errs.CodeToolExecError), err.Error(), false), nil
	}
	payload := cfg.Payload
	if payload == "" {
		payload = call.ArgumentsJSON
	}
	spec := requestSpec{
		Method:           http.MethodPost,
		URL:              cfg.URL,
		Headers:          cfg.Headers,
		Body:             payload,
		MaxResponseBytes: cfg.MaxResponseBytes,
		Retries:          cfg.Retries,
		RetryBackoffMS:   cfg.RetryBackoffMS,
	}
	encode := func(status int, payload []byte) (string, error) {
		return encodeWebhookResponse(status, payload, cfg.ResponseMap)
	}
	return executeRequest(ctx, w.Client, spec, invocationData(call, args), encode)
}

func validateResponseMap(mapping map[string]string) error {
	for key, path := range mapping {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("builtin:webhook: response_map key is required")
		}
		if path == "" {
			return fmt.Errorf("builtin:webhook: response_map[%q] path is required", key)
		}
		segments := strings.Split(path, ".")
		for _, segment := range segments {
			if segment == "" {
				return fmt.Errorf("builtin:webhook: response_map[%q] has an empty path segment", key)
			}
		}
		if segments[0] != "status" && segments[0] != "body" {
			return fmt.Errorf("builtin:webhook: response_map[%q] must start with status or body", key)
		}
	}
	return nil
}

func encodeWebhookResponse(status int, payload []byte, mapping map[string]string) (string, error) {
	if len(mapping) == 0 {
		return encodeBody(status, payload)
	}
	var body any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &body); err != nil {
			return "", fmt.Errorf("webhook response is not JSON: %w", err)
		}
	}
	root := map[string]any{
		"status": status,
		"body":   body,
	}
	result := make(map[string]any, len(mapping))
	for key, path := range mapping {
		value, ok := lookupPath(root, path)
		if !ok {
			return "", fmt.Errorf("webhook response path %q not found", path)
		}
		result[key] = value
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("encode webhook response: %w", err)
	}
	return string(encoded), nil
}

func lookupPath(root map[string]any, path string) (any, bool) {
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[segment]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}
