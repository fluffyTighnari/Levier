// Middleware — internal/register
//
// App registration, API key hashing, and authentication.
//
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

const apiKeyPrefix = "mw_"

const appColumns = `
	app_id, name, api_key_hash, multiagent, status, created_at, updated_at`

// Registry owns App registration and API-key authentication.
type Registry struct {
	pool    *pgxpool.Pool
	secrets SecretResolver
}

// New returns a PostgreSQL-backed registry.
func New(pool *pgxpool.Pool) *Registry {
	return &Registry{pool: pool, secrets: EnvSecretResolver{}}
}

// RegisterApp creates an App and returns the plaintext API key once.
// A duplicate name returns the existing App with an empty key.
func (r *Registry) RegisterApp(ctx context.Context, name string, multiagent bool) (runstore.App, string, bool, error) {
	if r == nil || r.pool == nil {
		return runstore.App{}, "", false, fmt.Errorf("register: postgres pool is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return runstore.App{}, "", false, fmt.Errorf("register: app name is required")
	}
	key, err := newAPIKey()
	if err != nil {
		return runstore.App{}, "", false, fmt.Errorf("register: generate api key: %w", err)
	}
	appID := "app_" + id.New()
	now := time.Now().UTC()
	app, err := scanApp(r.pool.QueryRow(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash, multiagent, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', $5, $6)
		ON CONFLICT (name) DO NOTHING
		RETURNING `+appColumns,
		appID, name, hashAPIKey(key), multiagent, now, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := r.GetAppByName(ctx, name)
		if lookupErr != nil {
			return runstore.App{}, "", false, lookupErr
		}
		return existing, "", false, nil
	}
	if err != nil {
		return runstore.App{}, "", false, fmt.Errorf("register: create app: %w", err)
	}
	return app, key, true, nil
}

// GetApp returns an App by ID.
func (r *Registry) GetApp(ctx context.Context, appID string) (runstore.App, error) {
	if r == nil || r.pool == nil {
		return runstore.App{}, fmt.Errorf("register: postgres pool is nil")
	}
	app, err := scanApp(r.pool.QueryRow(ctx,
		"SELECT "+appColumns+" FROM apps WHERE app_id = $1", appID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	if err != nil {
		return runstore.App{}, fmt.Errorf("register: get app: %w", err)
	}
	return app, nil
}

// GetAppByName returns an App by unique name.
func (r *Registry) GetAppByName(ctx context.Context, name string) (runstore.App, error) {
	if r == nil || r.pool == nil {
		return runstore.App{}, fmt.Errorf("register: postgres pool is nil")
	}
	app, err := scanApp(r.pool.QueryRow(ctx,
		"SELECT "+appColumns+" FROM apps WHERE name = $1", name,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	if err != nil {
		return runstore.App{}, fmt.Errorf("register: get app by name: %w", err)
	}
	return app, nil
}

// AuthenticateAPIKey returns the active App for a plaintext API key.
func (r *Registry) AuthenticateAPIKey(ctx context.Context, key string) (runstore.App, error) {
	if r == nil || r.pool == nil {
		return runstore.App{}, fmt.Errorf("register: postgres pool is nil")
	}
	if key == "" {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	app, err := scanApp(r.pool.QueryRow(ctx, `
		SELECT `+appColumns+`
		FROM apps
		WHERE api_key_hash = $1 AND status = 'active'
	`, hashAPIKey(key)))
	if errors.Is(err, pgx.ErrNoRows) {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	if err != nil {
		return runstore.App{}, fmt.Errorf("register: authenticate app: %w", err)
	}
	return app, nil
}

// UpdateApp updates the named App's mutable fields.
func (r *Registry) UpdateApp(ctx context.Context, appID, name string, multiagent bool) (runstore.App, error) {
	if r == nil || r.pool == nil {
		return runstore.App{}, fmt.Errorf("register: postgres pool is nil")
	}
	if appID == "" || strings.TrimSpace(name) == "" {
		return runstore.App{}, fmt.Errorf("register: app_id and name are required")
	}
	app, err := scanApp(r.pool.QueryRow(ctx, `
		UPDATE apps
		SET name = $2, multiagent = $3, updated_at = now()
		WHERE app_id = $1
		RETURNING `+appColumns,
		appID, strings.TrimSpace(name), multiagent,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	if err != nil {
		return runstore.App{}, fmt.Errorf("register: update app: %w", err)
	}
	return app, nil
}

// RotateAPIKey replaces an App's key and returns the plaintext once.
func (r *Registry) RotateAPIKey(ctx context.Context, appID string) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("register: postgres pool is nil")
	}
	key, err := newAPIKey()
	if err != nil {
		return "", fmt.Errorf("register: generate api key: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE apps SET api_key_hash = $2, updated_at = now()
		WHERE app_id = $1
	`, appID, hashAPIKey(key))
	if err != nil {
		return "", fmt.Errorf("register: rotate api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", runstore.ErrAppNotFound
	}
	return key, nil
}

func scanApp(row pgx.Row) (runstore.App, error) {
	var app runstore.App
	if err := row.Scan(
		&app.AppID,
		&app.Name,
		&app.APIKeyHash,
		&app.Multiagent,
		&app.Status,
		&app.CreatedAt,
		&app.UpdatedAt,
	); err != nil {
		return runstore.App{}, err
	}
	return app, nil
}

func newAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
