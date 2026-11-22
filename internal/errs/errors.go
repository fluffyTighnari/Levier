// Package errs defines the middleware error model and retry classification.
//
// Levier — internal/errs
// Copyright 2026 The Levier Authors.
package errs

import (
	"errors"
	"fmt"
)

// Code is a stable middleware error code.
type Code string

const (
	CodeNoDaemonAvailable            Code = "NO_DAEMON_AVAILABLE"
	CodeQueueOverflow                Code = "QUEUE_OVERFLOW"
	CodeQuotaExceeded                Code = "QUOTA_EXCEEDED"
	CodeQueueUnavailable             Code = "QUEUE_UNAVAILABLE"
	CodeToolNotFound                 Code = "TOOL_NOT_FOUND"
	CodeToolTimeout                  Code = "TOOL_TIMEOUT"
	CodeToolDenied                   Code = "TOOL_DENIED"
	CodeToolExecError                Code = "TOOL_EXEC_ERROR"
	CodeAppUnreachable               Code = "APP_UNREACHABLE"
	CodeRunTimeout                   Code = "RUN_TIMEOUT"
	CodeRunCancelled                 Code = "RUN_CANCELLED"
	CodeDaemonLost                   Code = "DAEMON_LOST"
	CodeMaxAttemptsExceeded          Code = "MAX_ATTEMPTS_EXCEEDED"
	CodeBadEnvelope                  Code = "BAD_ENVELOPE"
	CodeSeqGap                       Code = "SEQ_GAP"
	CodeAuthFailed                   Code = "AUTH_FAILED"
	CodeTokenExpired                 Code = "TOKEN_EXPIRED"
	CodeWorkflowTaskDead             Code = "WORKFLOW_TASK_DEAD"
	CodeWorkflowLeaseLost            Code = "WORKFLOW_LEASE_LOST"
	CodeInternal                     Code = "INTERNAL"
	CodeBadRequest                   Code = "BAD_REQUEST"
	CodeUnauthorized                 Code = "UNAUTHORIZED"
	CodeForbidden                    Code = "FORBIDDEN"
	CodeNotImplemented               Code = "NOT_IMPLEMENTED"
	CodeDependencyUnavailable        Code = "DEPENDENCY_UNAVAILABLE"
	CodeAppNotFound                  Code = "APP_NOT_FOUND"
	CodeAppNameConflict              Code = "APP_NAME_CONFLICT"
	CodeRunNotFound                  Code = "RUN_NOT_FOUND"
	CodeSessionNotFound              Code = "SESSION_NOT_FOUND"
	CodeIdempotencyConflict          Code = "IDEMPOTENCY_CONFLICT"
	CodeBadTool                      Code = "BAD_TOOL"
	CodeBadMCP                       Code = "BAD_MCP"
	CodeBadMCPRef                    Code = "BAD_MCP_REF"
	CodeBadModelProvider             Code = "BAD_MODEL_PROVIDER"
	CodeModelProviderNotFound        Code = "MODEL_PROVIDER_NOT_FOUND"
	CodeMCPNotFound                  Code = "MCP_NOT_FOUND"
	CodeMCPInUse                     Code = "MCP_IN_USE"
	CodeBadMessage                   Code = "BAD_MESSAGE"
	CodeReplayFailed                 Code = "REPLAY_FAILED"
	CodeResponseTooLarge             Code = "RESPONSE_TOO_LARGE"
	CodeRequestTooLarge              Code = "REQUEST_TOO_LARGE"
	CodeToolNotAvailable             Code = "TOOL_NOT_AVAILABLE"
	CodeToolBusy                     Code = "TOOL_BUSY"
	CodeLocalToolUnknown             Code = "LOCAL_TOOL_UNKNOWN"
	CodeLocalToolForbidden           Code = "LOCAL_TOOL_FORBIDDEN"
	CodeLocalToolCorrelationMismatch Code = "LOCAL_TOOL_CORRELATION_MISMATCH"
)

// Error is the canonical middleware error.
type Error struct {
	Code      Code
	Message   string
	Retryable bool
	Cause     error
}

// New creates an error using the default retryability for code.
func New(code Code, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: codeMetaByCode[code].retryable,
	}
}

// Wrap creates an error with a cause.
func Wrap(code Code, message string, cause error) *Error {
	err := New(code, message)
	err.Cause = cause
	return err
}

// WithRetryable overrides the retryability classification.
func (e *Error) WithRetryable(retryable bool) *Error {
	e.Retryable = retryable
	return e
}

// Error implements error.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetryable reports whether err is a middleware error marked retryable.
func IsRetryable(err error) bool {
	var target *Error
	if errors.As(err, &target) {
		return target.Retryable
	}
	return false
}

// CodeOf returns the middleware code, or empty for nil/unknown errors.
func CodeOf(err error) Code {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

// HTTPStatus maps an error code to an HTTP status.
func HTTPStatus(code Code) int {
	if meta, ok := codeMetaByCode[code]; ok {
		return meta.status
	}
	return 500
}

// HTTPStatusOf maps an error to an HTTP status.
func HTTPStatusOf(err error) int {
	return HTTPStatus(CodeOf(err))
}

type codeMeta struct {
	retryable bool
	status    int
}

var codeMetaByCode = map[Code]codeMeta{
	CodeNoDaemonAvailable:            {retryable: true, status: 503},
	CodeQueueOverflow:                {retryable: false, status: 429},
	CodeQuotaExceeded:                {retryable: false, status: 429},
	CodeQueueUnavailable:             {retryable: true, status: 503},
	CodeToolNotFound:                 {retryable: false, status: 404},
	CodeToolTimeout:                  {retryable: true, status: 504},
	CodeToolDenied:                   {retryable: false, status: 403},
	CodeToolExecError:                {retryable: false, status: 502},
	CodeAppUnreachable:               {retryable: true, status: 503},
	CodeRunTimeout:                   {retryable: false, status: 504},
	CodeRunCancelled:                 {retryable: false, status: 409},
	CodeDaemonLost:                   {retryable: false, status: 503},
	CodeMaxAttemptsExceeded:          {retryable: false, status: 500},
	CodeBadEnvelope:                  {retryable: false, status: 400},
	CodeSeqGap:                       {retryable: false, status: 500},
	CodeAuthFailed:                   {retryable: false, status: 401},
	CodeTokenExpired:                 {retryable: false, status: 401},
	CodeWorkflowTaskDead:             {retryable: false, status: 500},
	CodeWorkflowLeaseLost:            {retryable: true, status: 409},
	CodeInternal:                     {retryable: false, status: 500},
	CodeBadRequest:                   {retryable: false, status: 400},
	CodeUnauthorized:                 {retryable: false, status: 401},
	CodeForbidden:                    {retryable: false, status: 403},
	CodeNotImplemented:               {retryable: false, status: 501},
	CodeDependencyUnavailable:        {retryable: true, status: 503},
	CodeAppNotFound:                  {retryable: false, status: 404},
	CodeAppNameConflict:              {retryable: false, status: 409},
	CodeRunNotFound:                  {retryable: false, status: 404},
	CodeSessionNotFound:              {retryable: false, status: 404},
	CodeIdempotencyConflict:          {retryable: false, status: 409},
	CodeBadTool:                      {retryable: false, status: 400},
	CodeBadMCP:                       {retryable: false, status: 400},
	CodeBadMCPRef:                    {retryable: false, status: 400},
	CodeBadModelProvider:             {retryable: false, status: 400},
	CodeModelProviderNotFound:        {retryable: false, status: 404},
	CodeMCPNotFound:                  {retryable: false, status: 404},
	CodeMCPInUse:                     {retryable: false, status: 409},
	CodeBadMessage:                   {retryable: false, status: 400},
	CodeReplayFailed:                 {retryable: false, status: 500},
	CodeResponseTooLarge:             {retryable: false, status: 413},
	CodeToolNotAvailable:             {retryable: false, status: 503},
	CodeToolBusy:                     {retryable: true, status: 409},
	CodeLocalToolUnknown:             {retryable: false, status: 404},
	CodeLocalToolForbidden:           {retryable: false, status: 403},
	CodeLocalToolCorrelationMismatch: {retryable: false, status: 409},
	CodeRequestTooLarge:              {retryable: false, status: 413},
}
