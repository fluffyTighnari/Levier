// Middleware — connection
//
// Split from daemonhub.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package connection

import (
	"sort"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

// Reserve reserves one run slot on a daemon.
func (h *Hub) Reserve(daemonID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn := h.daemons[daemonID]
	if conn == nil {
		return false
	}
	maxRuns := conn.info.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	if effectiveRunning(conn.info) >= maxRuns {
		return false
	}
	conn.info.Reserved++
	return true
}

// Release releases one reserved run slot.
func (h *Hub) Release(daemonID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn := h.daemons[daemonID]
	if conn != nil && conn.info.Reserved > 0 {
		conn.info.Reserved--
	}
}

// Get returns a daemon registration snapshot.
func (h *Hub) Get(daemonID string) (DaemonInfo, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn := h.daemons[daemonID]
	if conn == nil {
		return DaemonInfo{}, false
	}
	info := cloneInfo(conn.info)
	info.Healthy = h.healthy(conn.info, h.cfg.Now())
	return info, true
}

// List returns registrations sorted by daemon_id.
func (h *Hub) List() []DaemonInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := h.cfg.Now()
	result := make([]DaemonInfo, 0, len(h.daemons))
	for _, conn := range h.daemons {
		info := cloneInfo(conn.info)
		info.Healthy = h.healthy(conn.info, now)
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DaemonID < result[j].DaemonID
	})
	return result
}

// Count returns the number of active daemon streams.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.daemons)
}

// PickHealthy returns the least-loaded matching daemon.
func (h *Hub) PickHealthy(
	runtimeType runtimev1.RuntimeType,
	strategy string,
	labels map[string]string,
) (string, bool) {
	now := h.cfg.Now()
	h.mu.RLock()
	defer h.mu.RUnlock()

	bestID := ""
	bestScore := 0.0
	for _, conn := range h.daemons {
		info := conn.info
		if !h.healthy(info, now) {
			continue
		}
		if runtimeType != runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED &&
			info.RuntimeType != runtimeType {
			continue
		}
		if strategy != "" && !contains(info.SupportedStrategies, strategy) {
			continue
		}
		if !labelsMatch(info.Labels, labels) {
			continue
		}
		maxRuns := info.MaxConcurrentRuns
		if maxRuns <= 0 {
			maxRuns = 1
		}
		if effectiveRunning(info) >= maxRuns {
			continue
		}
		score := float64(effectiveRunning(info)) / float64(maxRuns)
		if bestID == "" || score < bestScore {
			bestID = info.DaemonID
			bestScore = score
		}
	}
	return bestID, bestID != ""
}

// Sweep removes daemons whose last heartbeat exceeds HeartbeatTimeout.
func (h *Hub) Sweep(now time.Time) {
	h.mu.Lock()
	stale := make([]*daemonConn, 0)
	for daemonID, conn := range h.daemons {
		last := conn.info.LastHeartbeat
		if last.IsZero() {
			last = conn.info.ConnectedAt
		}
		if now.Sub(last) > h.cfg.HeartbeatTimeout {
			delete(h.daemons, daemonID)
			stale = append(stale, conn)
		}
	}
	h.mu.Unlock()
	for _, conn := range stale {
		conn.close()
		h.notifyOffline(conn.info.DaemonID)
	}
}

func (h *Hub) reaper() {
	ticker := time.NewTicker(h.cfg.HeartbeatCheckPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.Sweep(h.cfg.Now())
		}
	}
}

func cloneInfo(info DaemonInfo) DaemonInfo {
	info.SupportedStrategies = append([]string(nil), info.SupportedStrategies...)
	info.Labels = cloneStringMap(info.Labels)
	info.RunningRuns = append([]RunRef(nil), info.RunningRuns...)
	return info
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func labelsMatch(have, want map[string]string) bool {
	for key, value := range want {
		if have[key] != value {
			return false
		}
	}
	return true
}
