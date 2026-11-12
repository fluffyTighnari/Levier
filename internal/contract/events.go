// Middleware — event payload field names used by Control.
//
// The harness emits these keys; Control reads them when projecting metrics
// and terminal state. Keeping them here avoids string drift on the Go side.
//
// Copyright 2026 The Middleware Authors.
package contract

const (
	// EventFieldStatus is the RUN_STATUS payload status.
	EventFieldStatus = "status"
	// EventFieldInputTokens is the USAGE input token count.
	EventFieldInputTokens = "input_tokens"
	// EventFieldOutputTokens is the USAGE output token count.
	EventFieldOutputTokens = "output_tokens"
)
