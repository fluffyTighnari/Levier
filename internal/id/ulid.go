// Package id generates ULIDs and other middleware identifiers.
//
// Middleware — internal/id
// Copyright 2026 The Middleware Authors.
package id

import (
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// New returns a new ULID string using the current time.
func New() string {
	return ulid.Make().String()
}

// NewAt returns a new ULID string with an explicit timestamp.
func NewAt(t time.Time) (string, error) {
	value, err := ulid.New(ulid.Timestamp(t.UTC()), ulid.DefaultEntropy())
	if err != nil {
		return "", fmt.Errorf("new ulid: %w", err)
	}
	return value.String(), nil
}

// Parse parses a strict 26-character ULID string.
func Parse(value string) (ulid.ULID, error) {
	parsed, err := ulid.ParseStrict(value)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("parse ulid %q: %w", value, err)
	}
	return parsed, nil
}

// Timestamp returns the UTC timestamp encoded in a ULID string.
func Timestamp(value string) (time.Time, error) {
	parsed, err := Parse(value)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(int64(parsed.Time())).UTC(), nil
}
