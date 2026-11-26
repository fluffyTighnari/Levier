package version

import (
	"runtime/debug"
	"testing"
)

func TestApplyBuildInfoForGoInstall(t *testing.T) {
	oldVersion, oldCommit, oldBuilt := Version, Commit, Built
	defer func() {
		Version, Commit, Built = oldVersion, oldCommit, oldBuilt
	}()

	Version = "dev"
	Commit = "unknown"
	Built = "unknown"
	applyBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0-rc.5"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.time", Value: "2026-11-18T00:00:00Z"},
		},
	})

	if Version != "v0.1.0-rc.5" {
		t.Fatalf("Version = %q", Version)
	}
	if Commit != "abcdef1" {
		t.Fatalf("Commit = %q", Commit)
	}
	if Built != "2026-11-18T00:00:00Z" {
		t.Fatalf("Built = %q", Built)
	}
}

func TestApplyBuildInfoDoesNotOverrideLdflags(t *testing.T) {
	oldVersion, oldCommit, oldBuilt := Version, Commit, Built
	defer func() {
		Version, Commit, Built = oldVersion, oldCommit, oldBuilt
	}()

	Version = "v0.1.0"
	Commit = "abc1234"
	Built = "2026-11-17T00:00:00Z"
	applyBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "9999999999"},
			{Key: "vcs.time", Value: "2099-01-01T00:00:00Z"},
		},
	})

	if Version != "v0.1.0" || Commit != "abc1234" || Built != "2026-11-17T00:00:00Z" {
		t.Fatalf("metadata overridden: %q %q %q", Version, Commit, Built)
	}
}

func TestApplyBuildInfoIgnoresDevel(t *testing.T) {
	oldVersion, oldCommit, oldBuilt := Version, Commit, Built
	defer func() {
		Version, Commit, Built = oldVersion, oldCommit, oldBuilt
	}()

	Version = "dev"
	Commit = "unknown"
	Built = "unknown"
	applyBuildInfo(&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}})

	if Version != "dev" || Commit != "unknown" || Built != "unknown" {
		t.Fatalf("metadata = %q %q %q", Version, Commit, Built)
	}
}
