// Middleware — internal/runstore
//
// Run status constants and legal transitions.
//
// Copyright 2026 The Middleware Authors.
package runstore

const (
	// StatusQueued waits for dispatch.
	StatusQueued = "queued"
	// StatusAssigned is assigned to a daemon.
	StatusAssigned = "assigned"
	// StatusRunning is executing on a daemon.
	StatusRunning = "running"
	// StatusCompleted finished successfully.
	StatusCompleted = "completed"
	// StatusFailed finished with an error.
	StatusFailed = "failed"
	// StatusCancelled was cancelled.
	StatusCancelled = "cancelled"
	// StatusTimeout exceeded its deadline.
	StatusTimeout = "timeout"
)

var runTransitions = map[string]map[string]struct{}{
	StatusQueued: {
		StatusCancelled: {},
		StatusFailed:    {},
		StatusTimeout:   {},
	},
	StatusAssigned: {
		StatusRunning:   {},
		StatusCompleted: {},
		StatusFailed:    {},
		StatusCancelled: {},
		StatusTimeout:   {},
	},
	StatusRunning: {
		StatusCompleted: {},
		StatusFailed:    {},
		StatusCancelled: {},
		StatusTimeout:   {},
	},
}

// TerminalStatus reports whether status is final.
func TerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusTimeout:
		return true
	default:
		return false
	}
}

func canTransition(from, to string) bool {
	targets, ok := runTransitions[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}
