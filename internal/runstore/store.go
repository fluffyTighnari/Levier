// Levier — runstore
//
// Split from store.go (P8R.3).
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists apps, runs, and events in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a PostgreSQL-backed RunStore.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getRunByIdempotency(ctx context.Context, q querier, appID, key string) (Run, error) {
	run, err := scanRun(q.QueryRow(ctx,
		"SELECT "+runColumns+" FROM runs WHERE app_id = $1 AND idempotency_key = $2",
		appID, key,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("get run by idempotency: %w", err)
	}
	return run, nil
}
