// Middleware — internal/observe
//
// Secret redaction helpers for logs and diagnostics.
//
// Copyright 2026 The Middleware Authors.
package observe

import (
	"strings"
)

// Redacted is the replacement emitted for sensitive values.
const Redacted = "[REDACTED]"

var sensitiveKeys = []string{
	"authorization",
	"api_key",
	"apikey",
	"bearer",
	"cookie",
	"password",
	"secret",
	"set-cookie",
	"token",
	"x-api-key",
}

// RedactValue returns Redacted for sensitive keys and the original value otherwise.
func RedactValue(key, value string) string {
	if value == "" {
		return value
	}
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, sensitive := range sensitiveKeys {
		if lower == sensitive || strings.Contains(lower, sensitive) {
			return Redacted
		}
	}
	return value
}

// RedactMap copies values with sensitive keys replaced.
func RedactMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		redacted[key] = RedactValue(key, value)
	}
	return redacted
}
