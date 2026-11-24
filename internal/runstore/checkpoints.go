// Levier - longterm session checkpoint persistence.
// Copyright 2026 The Levier Authors.

package runstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SaveCheckpoint stores the latest opaque state for one session.
func (s *Store) SaveCheckpoint(ctx context.Context, checkpoint Checkpoint) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("runstore: postgres pool is nil")
	}
	if checkpoint.SessionID == "" || checkpoint.AppID == "" || checkpoint.TenantID == "" {
		return fmt.Errorf("%w: session_id, app_id, and tenant_id are required", ErrInvalidRun)
	}
	if len(checkpoint.State) == 0 {
		return fmt.Errorf("%w: checkpoint state is required", ErrInvalidRun)
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (
			session_id, app_id, tenant_id, status, current_run_id,
			checkpoint_blob, event_attempt, event_watermark, created_at, updated_at
		) VALUES (
			$1, $2, $3, 'active', NULLIF($4, ''),
			$5, $6, $7, now(), now()
		)
		ON CONFLICT (session_id) DO UPDATE SET
			current_run_id = EXCLUDED.current_run_id,
			checkpoint_blob = EXCLUDED.checkpoint_blob,
			event_attempt = EXCLUDED.event_attempt,
			event_watermark = EXCLUDED.event_watermark,
			updated_at = now()
		WHERE sessions.app_id = EXCLUDED.app_id
		  AND sessions.tenant_id = EXCLUDED.tenant_id
	`, checkpoint.SessionID, checkpoint.AppID, checkpoint.TenantID,
		checkpoint.RunID, checkpoint.State, checkpoint.EventAttempt, checkpoint.EventWatermark)
	if err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCheckpointConflict
	}
	return nil
}

// GetCheckpoint returns the latest checkpoint for one session.
func (s *Store) GetCheckpoint(
	ctx context.Context,
	sessionID, appID, tenantID string,
) (Checkpoint, error) {
	if s == nil || s.pool == nil {
		return Checkpoint{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" || appID == "" || tenantID == "" {
		return Checkpoint{}, fmt.Errorf("%w: session_id, app_id, and tenant_id are required", ErrInvalidRun)
	}
	var checkpoint Checkpoint
	var runID pgtype.Text
	err := s.pool.QueryRow(ctx, `
		SELECT session_id, app_id, tenant_id, current_run_id,
		       checkpoint_blob, event_attempt, event_watermark, updated_at
		FROM sessions
		WHERE session_id = $1
		  AND app_id = $2
		  AND tenant_id = $3
		  AND checkpoint_blob IS NOT NULL
	`, sessionID, appID, tenantID).Scan(
		&checkpoint.SessionID,
		&checkpoint.AppID,
		&checkpoint.TenantID,
		&runID,
		&checkpoint.State,
		&checkpoint.EventAttempt,
		&checkpoint.EventWatermark,
		&checkpoint.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, ErrCheckpointNotFound
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("get checkpoint: %w", err)
	}
	if runID.Valid {
		checkpoint.RunID = runID.String
	}
	return checkpoint, nil
}
