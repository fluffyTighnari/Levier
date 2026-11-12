// Middleware — internal/gateway
//
// PostgreSQL tool invocation idempotency store.
//
// Copyright 2026 The Middleware Authors.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
)

const defaultInvocationStaleAfter = 5 * time.Minute

// StoredResult is a terminal tool result read from storage.
type StoredResult struct {
	Result *runtimev1.ToolCallResult
}

// InvocationStore is the idempotency persistence contract.
type InvocationStore interface {
	Begin(ctx context.Context, invocationID, runID, toolName string) (owner bool, stored *StoredResult, err error)
	Get(ctx context.Context, invocationID string) (*StoredResult, bool, error)
	Complete(ctx context.Context, invocationID string, result *runtimev1.ToolCallResult) error
	Fail(ctx context.Context, invocationID string, result *runtimev1.ToolCallResult) error
}

// InvocationStoreOption customizes a PostgresInvocationStore.
type InvocationStoreOption func(*PostgresInvocationStore)

// WithStaleAfter sets how long an in-flight claim is trusted before takeover.
func WithStaleAfter(staleAfter time.Duration) InvocationStoreOption {
	return func(store *PostgresInvocationStore) {
		if staleAfter > 0 {
			store.staleAfter = staleAfter
		}
	}
}

// PostgresInvocationStore implements InvocationStore on tool_invocations.
type PostgresInvocationStore struct {
	pool       *pgxpool.Pool
	staleAfter time.Duration
}

// NewPostgresInvocationStore returns a PostgreSQL invocation store.
func NewPostgresInvocationStore(pool *pgxpool.Pool, opts ...InvocationStoreOption) *PostgresInvocationStore {
	store := &PostgresInvocationStore{
		pool:       pool,
		staleAfter: defaultInvocationStaleAfter,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// Begin claims an invocation; false means another worker owns it or it is terminal.
func (s *PostgresInvocationStore) Begin(
	ctx context.Context,
	invocationID, runID, toolName string,
) (bool, *StoredResult, error) {
	if s == nil || s.pool == nil {
		return false, nil, fmt.Errorf("gateway: postgres pool is nil")
	}
	if invocationID == "" || runID == "" || toolName == "" {
		return false, nil, fmt.Errorf("gateway: invocation_id, run_id, and tool_name are required")
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO tool_invocations (invocation_id, run_id, tool_name, status, created_at)
		VALUES ($1, $2, $3, 'in_flight', now())
		ON CONFLICT (invocation_id) DO NOTHING
	`, invocationID, runID, toolName)
	if err != nil {
		return false, nil, fmt.Errorf("gateway: begin invocation: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil, nil
	}

	// A crashed worker can leave an in-flight row behind; only take it over
	// after the claim has gone stale.
	tag, err = s.pool.Exec(ctx, `
		UPDATE tool_invocations
		SET status = 'in_flight',
		    result_json = NULL,
		    error_json = NULL,
		    created_at = now(),
		    completed_at = NULL
		WHERE invocation_id = $1
		  AND status = 'in_flight'
		  AND created_at < now() - ($2 * interval '1 second')
	`, invocationID, s.staleAfter.Seconds())
	if err != nil {
		return false, nil, fmt.Errorf("gateway: reclaim stale invocation: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil, nil
	}

	stored, ok, err := s.Get(ctx, invocationID)
	if err != nil {
		return false, nil, err
	}
	if ok {
		return false, stored, nil
	}
	return false, nil, nil
}

// Get reads a terminal invocation result.
func (s *PostgresInvocationStore) Get(ctx context.Context, invocationID string) (*StoredResult, bool, error) {
	if s == nil || s.pool == nil {
		return nil, false, fmt.Errorf("gateway: postgres pool is nil")
	}
	var status string
	var resultJSON, errorJSON *string
	err := s.pool.QueryRow(ctx, `
		SELECT status, result_json, error_json
		FROM tool_invocations
		WHERE invocation_id = $1
	`, invocationID).Scan(&status, &resultJSON, &errorJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("gateway: get invocation: %w", err)
	}
	if status == "in_flight" {
		return nil, false, nil
	}
	result := &runtimev1.ToolCallResult{InvocationId: invocationID}
	if status == "completed" {
		result.Ok = true
		if resultJSON != nil {
			result.ResultJson = *resultJSON
		}
	} else {
		result.Ok = false
		result.Error = &runtimev1.ErrorInfo{Code: string(errs.CodeToolExecError), Message: "stored invocation failed"}
		if errorJSON != nil {
			var info struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				Retryable bool   `json:"retryable"`
			}
			if json.Unmarshal([]byte(*errorJSON), &info) == nil {
				result.Error.Code = info.Code
				result.Error.Message = info.Message
				result.Error.Retryable = info.Retryable
			}
		}
	}
	return &StoredResult{Result: result}, true, nil
}

// Complete stores a successful terminal result.
func (s *PostgresInvocationStore) Complete(ctx context.Context, invocationID string, result *runtimev1.ToolCallResult) error {
	if result == nil {
		return fmt.Errorf("gateway: result is required")
	}
	return s.finish(ctx, invocationID, "completed", result.GetResultJson(), nil)
}

// Fail stores a failed terminal result.
func (s *PostgresInvocationStore) Fail(ctx context.Context, invocationID string, result *runtimev1.ToolCallResult) error {
	if result == nil || result.GetError() == nil {
		return fmt.Errorf("gateway: failed result with error is required")
	}
	info := map[string]any{
		"code":      result.GetError().GetCode(),
		"message":   result.GetError().GetMessage(),
		"retryable": result.GetError().GetRetryable(),
	}
	encoded, _ := json.Marshal(info)
	return s.finish(ctx, invocationID, "failed", "", encoded)
}

func (s *PostgresInvocationStore) finish(ctx context.Context, invocationID, status, resultJSON string, errorJSON []byte) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("gateway: postgres pool is nil")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tool_invocations
		SET status = $2,
		    result_json = NULLIF($3, ''),
		    error_json = NULLIF($4, ''),
		    completed_at = $5
		WHERE invocation_id = $1
		  AND status = 'in_flight'
	`, invocationID, status, resultJSON, string(errorJSON), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("gateway: finish invocation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gateway: invocation %q not found or already finished", invocationID)
	}
	return nil
}
