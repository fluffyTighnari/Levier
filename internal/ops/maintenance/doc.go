// Package maintenance inspects and prunes terminal Levier data.
//
// Retention is intentionally conservative. Deleting one terminal run also
// removes its events, workflow tasks, and tool invocations through existing
// foreign-key cascades. Active runs and sessions with active runs are never
// removed.
//
// Copyright 2026 The Levier Authors.
package maintenance
