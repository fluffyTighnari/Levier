// Middleware — internal/workflow
//
// PostgreSQL RunTaskQueue enqueue path.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/middleware/internal/id"
)

const taskColumns = `
	task_id, run_id, task_type, status, available_at, attempt, max_attempts,
	lease_owner, lease_token, lease_expires_at, last_error, payload,
	created_at, updated_at`

const insertTaskSQL = `
	INSERT INTO run_tasks (
		task_id, run_id, task_type, status, available_at, attempt, max_attempts,
		lease_owner, lease_token, lease_expires_at, last_error, payload,
		created_at, updated_at
	) VALUES (
		$1, $2, $3, 'PENDING', $4, 0, $5,
		'', 0, NULL, '', $6,
		$7, $8
	)
	ON CONFLICT (run_id, task_type)
		WHERE status IN ('PENDING', 'LEASED')
		DO NOTHING
	RETURNING ` + taskColumns

const activeTaskSQL = `
	SELECT ` + taskColumns + `
	FROM run_tasks
	WHERE run_id = $1
	  AND task_type = $2
	  AND status IN ('PENDING', 'LEASED')
	LIMIT 1`

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresStore implements the RunTaskQueue on PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a PostgreSQL-backed task store.
func NewPostgres(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Enqueue inserts a pending task or returns the existing active task.
func (s *PostgresStore) Enqueue(ctx context.Context, task Task) (Task, error) {
	if s == nil || s.pool == nil {
		return Task{}, fmt.Errorf("workflow: postgres pool is nil")
	}
	return enqueue(ctx, s.pool, task)
}

// EnqueueTx inserts a pending task in an existing transaction.
func (s *PostgresStore) EnqueueTx(ctx context.Context, tx pgx.Tx, task Task) (Task, error) {
	if tx == nil {
		return Task{}, fmt.Errorf("workflow: transaction is nil")
	}
	return enqueue(ctx, tx, task)
}

func enqueue(ctx context.Context, q querier, task Task) (Task, error) {
	normalized, err := normalizeForEnqueue(task)
	if err != nil {
		return Task{}, err
	}

	payload := normalized.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Task{}, fmt.Errorf("%w: encode payload: %v", ErrInvalidTask, err)
	}

	// A conflict means another active task already exists. Retry once if
	// that task disappears between the insert and the follow-up select.
	for attempt := 0; attempt < 2; attempt++ {
		row := q.QueryRow(ctx,
			insertTaskSQL,
			normalized.ID,
			normalized.RunID,
			string(normalized.Type),
			normalized.AvailableAt,
			normalized.MaxAttempts,
			payloadJSON,
			normalized.CreatedAt,
			normalized.UpdatedAt,
		)
		inserted, err := scanTask(row)
		if err == nil {
			return inserted, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Task{}, fmt.Errorf("enqueue task: %w", err)
		}

		existing, err := getActiveTask(ctx, q, normalized.RunID, normalized.Type)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, ErrTaskNotFound) {
			return Task{}, err
		}
	}
	return Task{}, fmt.Errorf("enqueue task: active task disappeared")
}

func getActiveTask(ctx context.Context, q querier, runID string, taskType TaskType) (Task, error) {
	row := q.QueryRow(ctx, activeTaskSQL, runID, string(taskType))
	task, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	return task, err
}

func scanTask(row pgx.Row) (Task, error) {
	var (
		task         Task
		taskType     string
		status       string
		leaseToken   int64
		leaseExpires *time.Time
		payload      []byte
	)
	if err := row.Scan(
		&task.ID,
		&task.RunID,
		&taskType,
		&status,
		&task.AvailableAt,
		&task.Attempt,
		&task.MaxAttempts,
		&task.LeaseOwner,
		&leaseToken,
		&leaseExpires,
		&task.LastError,
		&payload,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return Task{}, err
	}
	task.Type = TaskType(taskType)
	task.Status = Status(status)
	task.LeaseToken = uint64(leaseToken)
	if leaseExpires != nil {
		task.LeaseExpiresAt = *leaseExpires
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &task.Payload); err != nil {
			return Task{}, fmt.Errorf("decode task payload: %w", err)
		}
	}
	return task, nil
}

func normalizeForEnqueue(task Task) (Task, error) {
	now := time.Now().UTC()
	if task.ID == "" {
		task.ID = id.New()
	}
	if task.Status == "" {
		task.Status = StatusPending
	}
	if task.Status != StatusPending {
		return Task{}, fmt.Errorf("%w: enqueue requires PENDING, got %s", ErrInvalidTask, task.Status)
	}
	if task.Attempt != 0 {
		return Task{}, fmt.Errorf("%w: new task attempt must be 0", ErrInvalidTask)
	}
	if task.LeaseOwner != "" || task.LeaseToken != 0 || !task.LeaseExpiresAt.IsZero() {
		return Task{}, fmt.Errorf("%w: new task must not carry a lease", ErrInvalidTask)
	}
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = task.Type.DefaultMaxAttempts()
	}
	if task.AvailableAt.IsZero() {
		task.AvailableAt = now
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = now
	}
	if err := task.Validate(); err != nil {
		return Task{}, fmt.Errorf("%w: %v", ErrInvalidTask, err)
	}
	return task, nil
}
