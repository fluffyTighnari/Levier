// Middleware — internal/workflow
//
// PostgreSQL claim, lease, ack/nack operations.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const claimTaskSQL = `
	WITH candidate AS (
		SELECT task_id
		FROM run_tasks
		WHERE status = 'PENDING'
		  AND available_at <= now()
		  AND attempt < max_attempts
		ORDER BY available_at ASC, created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	)
	UPDATE run_tasks t
	SET status = 'LEASED',
	    lease_owner = $1,
	    lease_token = t.lease_token + 1,
	    lease_expires_at = now() + make_interval(secs => $2),
	    attempt = t.attempt + 1,
	    updated_at = now()
	FROM candidate c
	WHERE t.task_id = c.task_id
	RETURNING t.task_id, t.run_id, t.task_type, t.status, t.available_at,
	          t.attempt, t.max_attempts, t.lease_owner, t.lease_token,
	          t.lease_expires_at, t.last_error, t.payload, t.created_at, t.updated_at`

const extendLeaseSQL = `
	UPDATE run_tasks
	SET lease_expires_at = now() + make_interval(secs => $4),
	    updated_at = now()
	WHERE task_id = $1
	  AND status = 'LEASED'
	  AND lease_owner = $2
	  AND lease_token = $3
	RETURNING lease_expires_at`

const ackTaskSQL = `
	UPDATE run_tasks
	SET status = 'DONE',
	    lease_owner = '',
	    lease_expires_at = NULL,
	    last_error = '',
	    updated_at = now()
	WHERE task_id = $1
	  AND status = 'LEASED'
	  AND lease_owner = $2
	  AND lease_token = $3`

const nackTaskSQL = `
	UPDATE run_tasks
	SET status = CASE WHEN attempt < max_attempts THEN 'PENDING' ELSE 'DEAD' END,
	    available_at = CASE WHEN attempt < max_attempts THEN $4 ELSE available_at END,
	    lease_owner = '',
	    lease_expires_at = NULL,
	    last_error = $5,
	    updated_at = now()
	WHERE task_id = $1
	  AND status = 'LEASED'
	  AND lease_owner = $2
	  AND lease_token = $3`

const failTaskSQL = `
	UPDATE run_tasks
	SET status = 'DEAD',
	    lease_owner = '',
	    lease_expires_at = NULL,
	    last_error = $4,
	    updated_at = now()
	WHERE task_id = $1
	  AND status = 'LEASED'
	  AND lease_owner = $2
	  AND lease_token = $3`

// Claim leases one ready task to owner for ttl.
func (s *PostgresStore) Claim(ctx context.Context, owner string, ttl time.Duration) (Task, Lease, error) {
	if s == nil || s.pool == nil {
		return Task{}, Lease{}, fmt.Errorf("workflow: postgres pool is nil")
	}
	if owner == "" {
		return Task{}, Lease{}, fmt.Errorf("%w: owner is required", ErrInvalidTask)
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	task, err := scanTask(s.pool.QueryRow(ctx, claimTaskSQL, owner, ttl.Seconds()))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, Lease{}, ErrNoTask
	}
	if err != nil {
		return Task{}, Lease{}, fmt.Errorf("claim task: %w", err)
	}
	lease := Lease{
		TaskID:    task.ID,
		Owner:     task.LeaseOwner,
		Token:     task.LeaseToken,
		ExpiresAt: task.LeaseExpiresAt,
	}
	return task, lease, nil
}

// Extend renews a lease owned by owner/token.
func (s *PostgresStore) Extend(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if s == nil || s.pool == nil {
		return Lease{}, fmt.Errorf("workflow: postgres pool is nil")
	}
	if err := validateLease(lease); err != nil {
		return Lease{}, err
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	var expires time.Time
	err := s.pool.QueryRow(ctx, extendLeaseSQL,
		lease.TaskID, lease.Owner, lease.Token, ttl.Seconds(),
	).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, s.leaseFailure(ctx, lease)
	}
	if err != nil {
		return Lease{}, fmt.Errorf("extend lease: %w", err)
	}
	lease.ExpiresAt = expires
	return lease, nil
}

// Ack marks a task DONE.
func (s *PostgresStore) Ack(ctx context.Context, task Task, lease Lease) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("workflow: postgres pool is nil")
	}
	if err := validateTaskLease(task, lease); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, ackTaskSQL, lease.TaskID, lease.Owner, lease.Token)
	if err != nil {
		return fmt.Errorf("ack task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.leaseFailure(ctx, lease)
	}
	return nil
}

// Nack returns a task to PENDING or DEAD when attempts are exhausted.
func (s *PostgresStore) Nack(ctx context.Context, task Task, lease Lease, retryAt time.Time, cause error) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("workflow: postgres pool is nil")
	}
	if err := validateTaskLease(task, lease); err != nil {
		return err
	}
	if retryAt.IsZero() {
		retryAt = time.Now().UTC()
	}
	tag, err := s.pool.Exec(ctx, nackTaskSQL,
		lease.TaskID, lease.Owner, lease.Token, retryAt, errorString(cause),
	)
	if err != nil {
		return fmt.Errorf("nack task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.leaseFailure(ctx, lease)
	}
	return nil
}

// Fail marks a task DEAD without retry.
func (s *PostgresStore) Fail(ctx context.Context, task Task, lease Lease, cause error) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("workflow: postgres pool is nil")
	}
	if err := validateTaskLease(task, lease); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, failTaskSQL,
		lease.TaskID, lease.Owner, lease.Token, errorString(cause),
	)
	if err != nil {
		return fmt.Errorf("fail task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.leaseFailure(ctx, lease)
	}
	return nil
}

func (s *PostgresStore) leaseFailure(ctx context.Context, lease Lease) error {
	var status string
	var owner string
	var token int64
	err := s.pool.QueryRow(ctx, `
		SELECT status, lease_owner, lease_token
		FROM run_tasks
		WHERE task_id = $1
	`, lease.TaskID).Scan(&status, &owner, &token)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect task lease: %w", err)
	}
	if Status(status).Terminal() {
		// A successful terminal write keeps the fencing token unchanged;
		// a cancel/reap increments it, so the old owner lost its lease.
		if lease.Token != uint64(token) {
			return fmt.Errorf("%w: task=%s owner=%q token=%d", ErrLeaseLost, status, owner, token)
		}
		return fmt.Errorf("%w: task is %s", ErrTaskTerminal, status)
	}
	return fmt.Errorf("%w: task=%s owner=%q token=%d", ErrLeaseLost, status, owner, token)
}

func validateLease(lease Lease) error {
	if lease.TaskID == "" {
		return fmt.Errorf("%w: task_id is required", ErrInvalidTask)
	}
	if lease.Owner == "" {
		return fmt.Errorf("%w: lease owner is required", ErrInvalidTask)
	}
	if lease.Token == 0 {
		return fmt.Errorf("%w: lease token is required", ErrInvalidTask)
	}
	return nil
}

func validateTaskLease(task Task, lease Lease) error {
	if err := validateLease(lease); err != nil {
		return err
	}
	if task.ID == "" {
		return fmt.Errorf("%w: task_id is required", ErrInvalidTask)
	}
	if task.ID != lease.TaskID {
		return fmt.Errorf("%w: task and lease IDs differ", ErrLeaseLost)
	}
	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
