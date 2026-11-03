// Middleware — internal/config
//
// Duration parsing helper shared by YAML and environment overrides.
//
// Copyright 2026 The Middleware Authors.
package config

import "time"

func timeParseDuration(value string) (Duration, error) {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return Duration(parsed), nil
}
