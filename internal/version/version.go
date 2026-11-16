// Package version carries build metadata injected at link time.
//
// Levier — internal/version
// Copyright 2026 The Levier Authors.
package version

var (
	// Version is the release version, for example v0.1.0.
	Version = "dev"
	// Commit is the short git commit.
	Commit = "unknown"
	// Built is the UTC build timestamp.
	Built = "unknown"
)
