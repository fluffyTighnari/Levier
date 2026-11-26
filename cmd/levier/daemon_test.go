package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/fluffyTighnari/levier/internal/runtime/daemoncheck"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestRunDaemonUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDaemon([]string{"nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runDaemon(nope) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown daemon command") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunDaemonHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDaemon([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runDaemon(help) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "daemon check") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunDaemonCheckHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDaemonCheck([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runDaemonCheck(--help) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "--daemon-id") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunDaemonCheckBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDaemonCheck([]string{"--nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runDaemonCheck(--nope) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunDaemonCheckUnexpectedArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDaemonCheck([]string{"extra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runDaemonCheck(extra) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected argument") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestDaemonCheckExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "http auth", err: &daemoncheck.HTTPError{Status: 401}, want: 3},
		{name: "http forbidden", err: &daemoncheck.HTTPError{Status: 403}, want: 3},
		{name: "http bad request", err: &daemoncheck.HTTPError{Status: 400}, want: 2},
		{name: "http conflict", err: &daemoncheck.HTTPError{Status: 409}, want: 2},
		{name: "http server", err: &daemoncheck.HTTPError{Status: 500}, want: 4},
		{name: "http network", err: &daemoncheck.HTTPError{Status: 0}, want: 4},
		{name: "grpc auth", err: grpcstatus.Error(codes.Unauthenticated, "bad token"), want: 3},
		{name: "grpc unavailable", err: grpcstatus.Error(codes.Unavailable, "down"), want: 4},
		{name: "grpc duplicate", err: grpcstatus.Error(codes.AlreadyExists, "dup"), want: 5},
		{name: "generic", err: errors.New("mismatch"), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := daemonCheckExitCode(test.err); got != test.want {
				t.Fatalf("daemonCheckExitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
