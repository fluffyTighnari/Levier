// Package roles defines the deployable roles of one Levier binary.
//
// A role is a lifecycle/composition boundary, not a separate service or
// database. The all role expands to every role for local and single-process
// deployments.
//
// Copyright 2026 The Levier Authors.
package roles

import "fmt"

const (
	// All selects every role.
	All = "all"
	// Control runs scheduling and workflow workers.
	Control = "control"
	// API serves the public HTTP API.
	API = "api"
	// RuntimeGateway owns daemon RuntimeChannel connections.
	RuntimeGateway = "runtime-gateway"
	// AppGateway owns App WebSocket connections and local tools.
	AppGateway = "app-gateway"
)

var allRoles = []string{Control, API, RuntimeGateway, AppGateway}

// Set is a validated set of deployable roles.
type Set struct {
	all   bool
	roles map[string]struct{}
}

// Parse validates and expands role names.
func Parse(values []string) (Set, error) {
	if len(values) == 0 {
		return Set{}, fmt.Errorf("roles must not be empty")
	}
	set := Set{roles: make(map[string]struct{}, len(values))}
	for _, value := range values {
		switch value {
		case All:
			set.all = true
		case Control, API, RuntimeGateway, AppGateway:
			set.roles[value] = struct{}{}
		default:
			return Set{}, fmt.Errorf("unknown role %q", value)
		}
	}
	if !set.all && len(set.roles) == 0 {
		return Set{}, fmt.Errorf("roles must include at least one role")
	}
	return set, nil
}

// Has reports whether a role is selected.
func (s Set) Has(role string) bool {
	if s.all {
		return true
	}
	_, ok := s.roles[role]
	return ok
}

// IsAll reports whether every role is selected.
func (s Set) IsAll() bool {
	return s.all
}

// List returns deterministic expanded role names.
func (s Set) List() []string {
	if s.all {
		return append([]string(nil), allRoles...)
	}
	result := make([]string, 0, len(allRoles))
	for _, role := range allRoles {
		if _, ok := s.roles[role]; ok {
			result = append(result, role)
		}
	}
	return result
}
