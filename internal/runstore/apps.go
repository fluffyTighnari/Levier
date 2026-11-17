// Levier — runstore
//
// Split from store.go (P8R.3).
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const appColumns = `
	app_id, name, api_key_hash, multiagent, status, created_at, updated_at`

// CreateApp upserts an app by app_id.
func (s *Store) CreateApp(ctx context.Context, app App) (App, error) {
	if s == nil || s.pool == nil {
		return App{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if app.AppID == "" || app.Name == "" || app.APIKeyHash == "" {
		return App{}, fmt.Errorf("runstore: app_id, name, and api_key_hash are required")
	}
	if app.Status == "" {
		app.Status = "active"
	}
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash, multiagent, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (app_id) DO UPDATE SET
			name = EXCLUDED.name,
			api_key_hash = EXCLUDED.api_key_hash,
			multiagent = EXCLUDED.multiagent,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
		RETURNING `+appColumns,
		app.AppID, app.Name, app.APIKeyHash, app.Multiagent, app.Status, now, now,
	)
	created, err := scanApp(row)
	if err != nil {
		return App{}, fmt.Errorf("create app: %w", err)
	}
	return created, nil
}

// GetApp returns one app by app_id.
func (s *Store) GetApp(ctx context.Context, appID string) (App, error) {
	if s == nil || s.pool == nil {
		return App{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	app, err := scanApp(s.pool.QueryRow(ctx,
		"SELECT "+appColumns+" FROM apps WHERE app_id = $1",
		appID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return App{}, ErrAppNotFound
	}
	if err != nil {
		return App{}, fmt.Errorf("get app: %w", err)
	}
	return app, nil
}

func scanApp(row pgx.Row) (App, error) {
	var app App
	if err := row.Scan(
		&app.AppID,
		&app.Name,
		&app.APIKeyHash,
		&app.Multiagent,
		&app.Status,
		&app.CreatedAt,
		&app.UpdatedAt,
	); err != nil {
		return App{}, err
	}
	return app, nil
}
