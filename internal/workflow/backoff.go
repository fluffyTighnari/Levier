// Levier — internal/workflow
//
// Deterministic exponential backoff helper.
//
// Copyright 2026 The Levier Authors.
package workflow

import "time"

// Backoff returns the delay before the next delivery attempt.
// attempt is 1-based; base is the first delay; max caps the result.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		base = 2 * time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	if attempt <= 1 {
		return base
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}
