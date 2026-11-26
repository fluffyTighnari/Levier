// Levier — internal/ops/maintenance
//
// Retention policy validation.
//
// Copyright 2026 The Levier Authors.
package maintenance

import (
	"errors"
	"fmt"
	"time"
)

// ErrRetentionDisabled reports an attempted destructive prune while retention
// is disabled.
var ErrRetentionDisabled = errors.New("retention is disabled")

// Policy describes the age after which terminal data may be removed.
type Policy struct {
	Enabled     bool
	RunDays     int
	SessionDays int
	BatchSize   int
}

// Normalize validates a policy and applies safe defaults for a programmatic
// caller that left fields empty.
func (p Policy) Normalize() (Policy, error) {
	if p.RunDays <= 0 {
		return Policy{}, fmt.Errorf("retention run days must be positive")
	}
	if p.SessionDays <= 0 {
		return Policy{}, fmt.Errorf("retention session days must be positive")
	}
	if p.BatchSize <= 0 {
		p.BatchSize = 1000
	}
	if p.BatchSize > 10000 {
		return Policy{}, fmt.Errorf("retention batch size must not exceed 10000")
	}
	return p, nil
}

func (p Policy) runCutoff(now time.Time) time.Time {
	return now.UTC().AddDate(0, 0, -p.RunDays)
}

func (p Policy) sessionCutoff(now time.Time) time.Time {
	return now.UTC().AddDate(0, 0, -p.SessionDays)
}
