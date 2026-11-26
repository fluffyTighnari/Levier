// Package testutil provides helpers shared by integration tests.
//
// Levier — internal/testing/testutil
// Copyright 2026 The Levier Authors.
package testutil

import "os"

// EnvOrLegacy returns the new environment value, falling back to the legacy
// name for one release.
func EnvOrLegacy(name, legacy string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return os.Getenv(legacy)
}
