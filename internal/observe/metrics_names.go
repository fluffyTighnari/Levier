// Middleware — internal/observe
//
// Canonical metric names used by the middleware.
//
// Copyright 2026 The Middleware Authors.
package observe

const (
	// MetricHTTPRequestsTotal counts HTTP requests.
	MetricHTTPRequestsTotal = "mw_http_requests_total"
	// MetricHTTPRequestDuration observes HTTP request duration in seconds.
	MetricHTTPRequestDuration = "mw_http_request_duration_seconds"
	// MetricWSConnections reports current App WebSocket connections.
	MetricWSConnections = "mw_ws_connections"
	// MetricRunsTotal counts terminal runs.
	MetricRunsTotal = "mw_runs_total"
	// MetricRunQueueDuration observes time from run creation to assignment.
	MetricRunQueueDuration = "mw_run_queue_duration_seconds"
	// MetricToolInvocationsTotal counts tool executions.
	MetricToolInvocationsTotal = "mw_tool_invocations_total"
	// MetricToolDuration observes tool execution duration in seconds.
	MetricToolDuration = "mw_tool_duration_seconds"
	// MetricEventsTotal counts persisted events.
	MetricEventsTotal = "mw_events_total"
	// MetricEventsDroppedTotal counts dropped or skipped events.
	MetricEventsDroppedTotal = "mw_events_dropped_total"
	// MetricWorkflowTasks reports task counts by type and status.
	MetricWorkflowTasks = "mw_workflow_tasks"
	// MetricWorkflowTaskAge reports the age of the oldest pending task per type.
	MetricWorkflowTaskAge = "mw_workflow_task_age_seconds"
	// MetricWorkflowClaimsTotal reports cumulative claims.
	MetricWorkflowClaimsTotal = "mw_workflow_claims_total"
	// MetricWorkflowAcksTotal reports cumulative acks.
	MetricWorkflowAcksTotal = "mw_workflow_acks_total"
	// MetricWorkflowNacksTotal reports cumulative nacks.
	MetricWorkflowNacksTotal = "mw_workflow_nacks_total"
	// MetricWorkflowFailsTotal reports cumulative failures.
	MetricWorkflowFailsTotal = "mw_workflow_fails_total"
	// MetricWorkflowDeadTotal reports cumulative dead-lettered tasks.
	MetricWorkflowDeadTotal = "mw_workflow_dead_total"
	// MetricWorkflowReapedTotal reports cumulative expired leases reclaimed.
	MetricWorkflowReapedTotal = "mw_workflow_reaped_total"
	// MetricWorkflowLeaseLostTotal reports cumulative lost leases.
	MetricWorkflowLeaseLostTotal = "mw_workflow_lease_lost_total"
	// MetricWorkflowPanicsTotal reports cumulative handler panics.
	MetricWorkflowPanicsTotal = "mw_workflow_panics_total"
	// MetricWorkflowClaimDuration reports average claim duration in seconds.
	MetricWorkflowClaimDuration = "mw_workflow_claim_duration_seconds"
	// MetricWorkflowHandlerDuration reports average handler duration in seconds.
	MetricWorkflowHandlerDuration = "mw_workflow_handler_duration_seconds"
	// MetricDaemonCapacity reports daemon capacity by kind (used/total).
	MetricDaemonCapacity = "mw_daemon_capacity"
	// MetricStaleDaemonEventsTotal counts dropped stale-epoch events.
	MetricStaleDaemonEventsTotal = "mw_stale_daemon_events_total"
	// MetricEventGapsTotal counts event sequence gaps.
	MetricEventGapsTotal = "mw_event_gaps_total"
	// MetricEphemeralEventsTotal counts skipped ephemeral events.
	MetricEphemeralEventsTotal = "mw_ephemeral_events_total"
	// MetricTokenUsageTotal counts token usage from USAGE events.
	MetricTokenUsageTotal = "mw_token_usage_total"
)
