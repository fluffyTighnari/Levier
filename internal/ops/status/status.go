// Levier — aggregate status overview.
//
// Copyright 2026 The Levier Authors.
package status

import (
	"context"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

const (
	// Version is the public status schema version.
	Version = 1
	// ServiceName is the public service identifier.
	ServiceName = "levier"
)

// Snapshot is one point-in-time status overview.
type Snapshot struct {
	StatusVersion int             `json:"status_version"`
	Service       string          `json:"service"`
	AsOf          string          `json:"as_of"`
	UptimeSeconds int64           `json:"uptime_seconds"`
	Build         Build           `json:"build"`
	Health        Health          `json:"health"`
	Daemons       DaemonSummary   `json:"daemons"`
	Apps          AppSummary      `json:"apps"`
	Runs          RunSummary      `json:"runs"`
	Workflow      WorkflowSummary `json:"workflow"`
}

// Build identifies the running binary.
type Build struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// Health contains process and dependency readiness.
type Health struct {
	Liveness  string `json:"liveness"`
	Readiness string `json:"readiness"`
	Database  string `json:"database"`
}

// DaemonSummary aggregates connected runtime capacity.
type DaemonSummary struct {
	Connected int                       `json:"connected"`
	Healthy   int                       `json:"healthy"`
	Capacity  int                       `json:"capacity"`
	Used      int                       `json:"used"`
	Available int                       `json:"available"`
	ByRuntime map[string]RuntimeSummary `json:"by_runtime"`
}

// RuntimeSummary aggregates one runtime type.
type RuntimeSummary struct {
	Connected int `json:"connected"`
	Healthy   int `json:"healthy"`
	Capacity  int `json:"capacity"`
	Used      int `json:"used"`
	Available int `json:"available"`
}

// AppSummary contains aggregate App state without identifiers.
type AppSummary struct {
	Registered           int `json:"registered"`
	WebSocketConnections int `json:"websocket_connections"`
}

// RunSummary contains aggregate Run state.
type RunSummary struct {
	Total    int64            `json:"total"`
	Active   int64            `json:"active"`
	ByStatus map[string]int64 `json:"by_status"`
}

// WorkflowSummary contains aggregate RunTaskQueue state.
type WorkflowSummary struct {
	Total                int64 `json:"total"`
	Pending              int64 `json:"pending"`
	Leased               int64 `json:"leased"`
	Done                 int64 `json:"done"`
	Dead                 int64 `json:"dead"`
	Cancelled            int64 `json:"cancelled"`
	OldestPendingSeconds int64 `json:"oldest_pending_seconds"`
}

// DaemonSource supplies connected daemon snapshots.
type DaemonSource interface {
	List() []connection.DaemonInfo
}

// AppSource supplies aggregate App connection statistics.
type AppSource interface {
	Stats() connection.AppStats
}

// RunSource supplies aggregate persisted Run statistics.
type RunSource interface {
	Stats(ctx context.Context) (runstore.Stats, error)
	CountApps(ctx context.Context) (int64, error)
}

// WorkflowSource supplies aggregate workflow queue statistics.
type WorkflowSource interface {
	Stats(ctx context.Context) (workflow.Stats, error)
}

// Options supplies dependencies and process metadata.
type Options struct {
	Hub       DaemonSource
	AppHub    AppSource
	Runs      RunSource
	Workflow  WorkflowSource
	Health    func(context.Context) error
	Version   string
	Commit    string
	StartedAt time.Time
	Now       func() time.Time
}

// Service builds status snapshots from live components.
type Service struct {
	opts Options
}

// New returns a status service with defaults applied.
func New(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Commit == "" {
		opts.Commit = "unknown"
	}
	return &Service{opts: opts}
}

// Snapshot returns one status snapshot. Partial source failures degrade
// readiness but do not make the overview unavailable.
func (s *Service) Snapshot(ctx context.Context) Snapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	now := s.opts.Now().UTC()
	snapshot := Snapshot{
		StatusVersion: Version,
		Service:       ServiceName,
		AsOf:          now.Format(time.RFC3339),
		Build: Build{
			Version: s.opts.Version,
			Commit:  s.opts.Commit,
		},
		Health: Health{
			Liveness:  "ok",
			Readiness: "unknown",
			Database:  "unknown",
		},
		Daemons: DaemonSummary{
			ByRuntime: make(map[string]RuntimeSummary),
		},
		Apps: AppSummary{},
		Runs: RunSummary{
			ByStatus: make(map[string]int64),
		},
		Workflow: WorkflowSummary{},
	}
	if !s.opts.StartedAt.IsZero() {
		seconds := int64(now.Sub(s.opts.StartedAt).Seconds())
		if seconds > 0 {
			snapshot.UptimeSeconds = seconds
		}
	}

	dbReady := false
	if s.opts.Health != nil {
		if err := s.opts.Health(ctx); err != nil {
			snapshot.Health.Readiness = "degraded"
			snapshot.Health.Database = "down"
		} else {
			dbReady = true
			snapshot.Health.Readiness = "ok"
			snapshot.Health.Database = "up"
		}
	} else {
		snapshot.Health.Readiness = "unknown"
	}

	s.collectDaemons(&snapshot)
	if s.opts.AppHub != nil {
		appStats := s.opts.AppHub.Stats()
		snapshot.Apps.WebSocketConnections = appStats.Connections
	}
	if dbReady && s.opts.Runs != nil {
		if stats, err := s.opts.Runs.Stats(ctx); err == nil {
			snapshot.Runs.Total = stats.Total
			for status, count := range stats.ByStatus {
				snapshot.Runs.ByStatus[status] = count
			}
			snapshot.Runs.Active = snapshot.Runs.ByStatus[runstore.StatusAssigned] + snapshot.Runs.ByStatus[runstore.StatusRunning]
		} else {
			snapshot.Health.Readiness = "degraded"
		}
		if count, err := s.opts.Runs.CountApps(ctx); err == nil {
			snapshot.Apps.Registered = int(count)
		} else {
			snapshot.Health.Readiness = "degraded"
		}
	}
	if dbReady && s.opts.Workflow != nil {
		if stats, err := s.opts.Workflow.Stats(ctx); err == nil {
			snapshot.Workflow = workflowSummary(stats)
		} else {
			snapshot.Health.Readiness = "degraded"
		}
	}
	initializeRunStatuses(snapshot.Runs.ByStatus)
	return snapshot
}

func (s *Service) collectDaemons(snapshot *Snapshot) {
	if s.opts.Hub == nil {
		return
	}
	for _, daemon := range s.opts.Hub.List() {
		runtimeName := daemonRuntimeName(daemon.RuntimeType.String())
		total := int(daemon.MaxConcurrentRuns)
		if total <= 0 {
			total = 1
		}
		used := int(daemon.RunningCount)
		if int(daemon.Reserved) > used {
			used = int(daemon.Reserved)
		}
		available := total - used
		if available < 0 {
			available = 0
		}

		snapshot.Daemons.Connected++
		if daemon.Healthy {
			snapshot.Daemons.Healthy++
		}
		snapshot.Daemons.Capacity += total
		snapshot.Daemons.Used += used
		snapshot.Daemons.Available += available

		runtimeSummary := snapshot.Daemons.ByRuntime[runtimeName]
		runtimeSummary.Connected++
		if daemon.Healthy {
			runtimeSummary.Healthy++
		}
		runtimeSummary.Capacity += total
		runtimeSummary.Used += used
		runtimeSummary.Available += available
		snapshot.Daemons.ByRuntime[runtimeName] = runtimeSummary
	}
}

func daemonRuntimeName(value string) string {
	name := strings.ToLower(value)
	name = strings.TrimPrefix(name, "runtime_type_")
	if name == "" || name == "unspecified" {
		return "unknown"
	}
	return name
}

func workflowSummary(stats workflow.Stats) WorkflowSummary {
	return WorkflowSummary{
		Total:                stats.Total,
		Pending:              stats.ByStatus[workflow.StatusPending],
		Leased:               stats.ByStatus[workflow.StatusLeased],
		Done:                 stats.ByStatus[workflow.StatusDone],
		Dead:                 stats.ByStatus[workflow.StatusDead],
		Cancelled:            stats.ByStatus[workflow.StatusCancelled],
		OldestPendingSeconds: int64(stats.OldestPendingAge.Seconds()),
	}
}

func initializeRunStatuses(statuses map[string]int64) {
	for _, status := range []string{
		runstore.StatusQueued,
		runstore.StatusAssigned,
		runstore.StatusRunning,
		runstore.StatusWaiting,
		runstore.StatusCompleted,
		runstore.StatusFailed,
		runstore.StatusCancelled,
		runstore.StatusTimeout,
	} {
		if _, ok := statuses[status]; !ok {
			statuses[status] = 0
		}
	}
}
