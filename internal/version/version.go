// Package version carries build metadata injected at link time.
//
// Levier — internal/version
// Copyright 2026 The Levier Authors.
package version

import (
	"runtime/debug"
	"strings"
)

var (
	// Version is the release version, for example v0.1.0.
	Version = "dev"
	// Commit is the short git commit.
	Commit = "unknown"
	// Built is the UTC build timestamp.
	Built = "unknown"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		applyBuildInfo(info)
	}
}

// applyBuildInfo fills version metadata when a binary was installed with
// `go install module@version` and therefore has no ldflags injection.
func applyBuildInfo(info *debug.BuildInfo) {
	if info == nil {
		return
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if Commit == "unknown" && setting.Value != "" {
				Commit = shortCommit(setting.Value)
			}
		case "vcs.time":
			if Built == "unknown" && setting.Value != "" {
				Built = setting.Value
			}
		}
	}
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 7 {
		return value[:7]
	}
	return value
}
