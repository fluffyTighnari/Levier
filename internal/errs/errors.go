// Package errs defines the middleware error model and retry classification.
//
// Middleware — internal/errs
// Copyright 2026 The Middleware Authors.
package errs

import (
	"errors"
	"fmt"
)

// Code is a stable middleware error code.
type Code string

const (
	CodeNoDaemonAvailable   Code = "NO_DAEMON_AVAILABLE"
	CodeQueueOverflow       Code = "QUEUE_OVERFLOW"
	CodeQueueUnavailable    Code = "QUEUE_UNAVAILABLE"
	CodeToolNotFound        Code = "TOOL_NOT_FOUND"
	CodeToolTimeout         Code = "TOOL_TIMEOUT"
	CodeToolDenied          Code = "TOOL_DENIED"
	CodeToolExecError       Code = "TOOL_EXEC_ERROR"
	CodeAppUnreachable      Code = "APP_UNREACHABLE"
	CodeRunTimeout          Code = "RUN_TIMEOUT"
	CodeRunCancelled        Code = "RUN_CANCELLED"
	CodeDaemonLost          Code = "DAEMON_LOST"
	CodeMaxAttemptsExceeded Code = "MAX_ATTEMPTS_EXCEEDED"
	CodeBadEnvelope         Code = "BAD_ENVELOPE"
	CodeSeqGap              Code = "SEQ_GAP"
	CodeAuthFailed          Code = "AUTH_FAILED"
	CodeTokenExpired        Code = "TOKEN_EXPIRED"
	CodeWorkflowTaskDead    Code = "WORKFLOW_TASK_DEAD"
	CodeWorkflowLeaseLost   Code = "WORKFLOW_LEASE_LOST"
	CodeInternal            Code = "INTERNAL"
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
		Retryable: retryableByCode[code],
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
	if status, ok := httpStatusByCode[code]; ok {
		return status
	}
	return 500
}

// HTTPStatusOf maps an error to an HTTP status.
func HTTPStatusOf(err error) int {
	return HTTPStatus(CodeOf(err))
}

var retryableByCode = map[Code]bool{
	CodeNoDaemonAvailable:   true,
	CodeQueueOverflow:       false,
	CodeQueueUnavailable:    true,
	CodeToolNotFound:        false,
	CodeToolTimeout:         true,
	CodeToolDenied:          false,
	CodeToolExecError:       false,
	CodeAppUnreachable:      true,
	CodeRunTimeout:          false,
	CodeRunCancelled:        false,
	CodeDaemonLost:          false,
	CodeMaxAttemptsExceeded: false,
	CodeBadEnvelope:         false,
	CodeSeqGap:              false,
	CodeAuthFailed:          false,
	CodeTokenExpired:        false,
	CodeWorkflowTaskDead:    false,
	CodeWorkflowLeaseLost:   true,
	CodeInternal:            false,
}

var httpStatusByCode = map[Code]int{
	CodeNoDaemonAvailable:   503,
	CodeQueueOverflow:       429,
	CodeQueueUnavailable:    503,
	CodeToolNotFound:        404,
	CodeToolTimeout:         504,
	CodeToolDenied:          403,
	CodeToolExecError:       502,
	CodeAppUnreachable:      503,
	CodeRunTimeout:          504,
	CodeRunCancelled:        409,
	CodeDaemonLost:          503,
	CodeMaxAttemptsExceeded: 500,
	CodeBadEnvelope:         400,
	CodeSeqGap:              500,
	CodeAuthFailed:          401,
	CodeTokenExpired:        401,
	CodeWorkflowTaskDead:    500,
	CodeWorkflowLeaseLost:   409,
	CodeInternal:            500,
}
