// Middleware — internal/control
//
// Run placement scheduler.
//
// Copyright 2026 The Middleware Authors.
package control

import (
	"slices"
	"sort"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/connection"
)

// PlacementRequest describes one scheduling decision.
type PlacementRequest struct {
	Runtime          runtimev1.RuntimeType
	Strategy         string
	Labels           map[string]string
	StickyDaemonID   string
	ExcludeDaemonIDs map[string]struct{}
}

// Candidate is one schedulable daemon snapshot.
type Candidate struct {
	DaemonID            string
	RuntimeType         runtimev1.RuntimeType
	SupportedStrategies []string
	Labels              map[string]string
	MaxConcurrentRuns   int32
	RunningCount        int32
	Healthy             bool
}

// Scheduler chooses a daemon for one placement request.
type Scheduler interface {
	Pick(request PlacementRequest) (daemonID string, ok bool)
}

// Select implements filtering, longterm stickiness, and least-loaded choice.
func Select(candidates []Candidate, request PlacementRequest) (string, bool) {
	eligible := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidateEligible(candidate, request) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		return "", false
	}
	for _, candidate := range eligible {
		if candidate.DaemonID == request.StickyDaemonID {
			return candidate.DaemonID, true
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left := loadRatio(eligible[i])
		right := loadRatio(eligible[j])
		if left == right {
			return eligible[i].DaemonID < eligible[j].DaemonID
		}
		return left < right
	})
	return eligible[0].DaemonID, true
}

// HubScheduler uses a live DaemonHub snapshot.
type HubScheduler struct {
	hub *connection.Hub
}

// NewHubScheduler returns a scheduler backed by the hub.
func NewHubScheduler(hub *connection.Hub) *HubScheduler {
	return &HubScheduler{hub: hub}
}

// Pick implements Scheduler.
func (s *HubScheduler) Pick(request PlacementRequest) (string, bool) {
	if s == nil || s.hub == nil {
		return "", false
	}
	infos := s.hub.List()
	candidates := make([]Candidate, 0, len(infos))
	for _, info := range infos {
		candidates = append(candidates, Candidate{
			DaemonID:            info.DaemonID,
			RuntimeType:         info.RuntimeType,
			SupportedStrategies: info.SupportedStrategies,
			Labels:              info.Labels,
			MaxConcurrentRuns:   info.MaxConcurrentRuns,
			RunningCount:        maxInt32(info.RunningCount, info.Reserved),
			Healthy:             info.Healthy,
		})
	}
	return Select(candidates, request)
}

func candidateEligible(candidate Candidate, request PlacementRequest) bool {
	if !candidate.Healthy {
		return false
	}
	if _, excluded := request.ExcludeDaemonIDs[candidate.DaemonID]; excluded {
		return false
	}
	if request.Runtime != runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED &&
		candidate.RuntimeType != request.Runtime {
		return false
	}
	if request.Strategy != "" && !contains(candidate.SupportedStrategies, request.Strategy) {
		return false
	}
	for key, value := range request.Labels {
		if candidate.Labels[key] != value {
			return false
		}
	}
	maxRuns := candidate.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	return candidate.RunningCount < maxRuns
}

func maxInt32(left, right int32) int32 {
	if left > right {
		return left
	}
	return right
}

func loadRatio(candidate Candidate) float64 {
	maxRuns := candidate.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	return float64(candidate.RunningCount) / float64(maxRuns)
}

func contains(values []string, target string) bool {
	return slices.Contains(values, target)
}
