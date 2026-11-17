// Levier — internal/config
//
// Duration parsing for human-readable YAML configuration.
//
// Copyright 2026 The Levier Authors.
package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from strings such as "30s".
type Duration time.Duration

// Std returns the standard-library duration value.
func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

// UnmarshalYAML decodes a duration string.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}

	*d = Duration(parsed)
	return nil
}

// MarshalYAML encodes the duration as a human-readable string.
func (d Duration) MarshalYAML() (any, error) {
	return d.Std().String(), nil
}
