package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestRunCLIVersionHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(version) = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"levier ", "commit:", "built:", "go:", "platform:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("version output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunCLIVersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"version", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(version --json) = %d, stderr = %s", code, stderr.String())
	}
	var info versionInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version JSON: %v (%s)", err, stdout.String())
	}
	if info.Version == "" || info.Go == "" || info.Platform == "" {
		t.Fatalf("incomplete version info: %+v", info)
	}
}

func TestRunCLIUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runCLI(nope) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: nope") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunCLIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI(help) = %d", code)
	}
	if !strings.Contains(stdout.String(), "levier serve") {
		t.Fatalf("usage = %s", stdout.String())
	}
}

func TestSubcommandHelp(t *testing.T) {
	tests := []struct {
		name string
		run  func([]string, io.Writer, io.Writer) int
	}{
		{name: "doctor", run: runDoctor},
		{name: "describe", run: runDescribe},
		{name: "selfcheck", run: runSelfcheck},
		{name: "status", run: runStatus},
		{name: "app init", run: runAppInit},
		{name: "app show", run: runAppShow},
		{name: "app rotate-key", run: runAppRotateKey},
		{name: "run submit", run: runRunSubmit},
		{name: "run watch", run: runRunWatch},
		{name: "run cancel", run: runRunCancel},
		{name: "migrate status", run: runMigrateStatus},
		{name: "migrate up", run: runMigrateUp},
		{name: "maintenance status", run: runMaintenanceStatus},
		{name: "maintenance prune", run: runMaintenancePrune},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := test.run([]string{"--help"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("%s --help = %d, stderr = %s", test.name, code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "usage: levier "+test.name) {
				t.Fatalf("%s --help stdout = %s", test.name, stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("%s --help stderr = %s", test.name, stderr.String())
			}
		})
	}
}

func TestSubcommandBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("doctor --nope = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: levier doctor") {
		t.Fatalf("stderr usage = %s", stderr.String())
	}
}

func TestHelpDoesNotLeakTokenDefaults(t *testing.T) {
	t.Setenv("LEVIER_ADMIN_TOKEN", "admin-super-secret")
	t.Setenv("LEVIER_DAEMON_TOKEN", "daemon-super-secret")

	var stdout, stderr bytes.Buffer
	if code := runAppInit([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("app init --help = %d", code)
	}
	if strings.Contains(stdout.String(), "admin-super-secret") {
		t.Fatalf("app init help leaked admin token: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runDaemonCheck([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("daemon check --help = %d", code)
	}
	if strings.Contains(stdout.String(), "daemon-super-secret") || strings.Contains(stdout.String(), "admin-super-secret") {
		t.Fatalf("daemon check help leaked token: %s", stdout.String())
	}
}
