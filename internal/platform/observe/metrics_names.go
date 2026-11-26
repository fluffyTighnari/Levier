// Levier — internal/platform/observe
//
// Canonical metric names used by the middleware.
//
// Copyright 2026 The Levier Authors.
package observe

const (
	// MetricHTTPRequestsTotal counts HTTP requests.
	MetricHTTPRequestsTotal = "levier_http_requests_total"
	// MetricHTTPRequestDuration observes HTTP request duration in seconds.
	MetricHTTPRequestDuration = "levier_http_request_duration_seconds"
	// MetricWSConnections reports current App WebSocket connections.
	MetricWSConnections = "levier_ws_connections"
	// MetricRunsTotal counts terminal runs.
	MetricRunsTotal = "levier_runs_total"
	// MetricRuntimeCapabilities reports one advertised Runtime capability.
	MetricRuntimeCapabilities = "levier_runtime_capabilities"
	// MetricRunQueueDuration observes time from run creation to assignment.
	MetricRunQueueDuration = "levier_run_queue_duration_seconds"
	// MetricToolInvocationsTotal counts tool executions.
	MetricToolInvocationsTotal = "levier_tool_invocations_total"
	// MetricToolDuration observes tool execution duration in seconds.
	MetricToolDuration = "levier_tool_duration_seconds"
	// MetricEventsTotal counts persisted events.
	MetricEventsTotal = "levier_events_total"
	// MetricCustomEventsTotal counts persisted custom events by name.
	MetricCustomEventsTotal = "levier_custom_events_total"
	// MetricCheckpointsSavedTotal counts persisted longterm checkpoints.
	MetricCheckpointsSavedTotal = "levier_checkpoints_saved_total"
	// MetricCheckpointSizeBytes observes serialized checkpoint size.
	MetricCheckpointSizeBytes = "levier_checkpoint_size_bytes"
	// MetricCheckpointRestoreTotal counts checkpoint restore outcomes.
	MetricCheckpointRestoreTotal = "levier_checkpoint_restore_total"
	// MetricRunWaitingSeconds observes time spent in the waiting state.
	MetricRunWaitingSeconds = "levier_run_waiting_seconds"
	// MetricRunResumeTotal counts waiting runs resumed by an operator or App.
	MetricRunResumeTotal = "levier_run_resume_total"
	// MetricEventsDroppedTotal counts dropped or skipped events.
	MetricEventsDroppedTotal = "levier_events_dropped_total"
	// MetricEventAppendDuration observes run event persistence latency.
	MetricEventAppendDuration = "levier_event_append_duration_seconds"
	// MetricEventAppendErrorsTotal counts failed run event inserts.
	MetricEventAppendErrorsTotal = "levier_event_append_errors_total"
	// MetricWorkflowTasks reports task counts by type and status.
	MetricWorkflowTasks = "levier_workflow_tasks"
	// MetricWorkflowTaskAge reports the age of the oldest pending task per type.
	MetricWorkflowTaskAge = "levier_workflow_task_age_seconds"
	// MetricWorkflowClaimsTotal reports cumulative claims.
	MetricWorkflowClaimsTotal = "levier_workflow_claims_total"
	// MetricWorkflowAcksTotal reports cumulative acks.
	MetricWorkflowAcksTotal = "levier_workflow_acks_total"
	// MetricWorkflowNacksTotal reports cumulative nacks.
	MetricWorkflowNacksTotal = "levier_workflow_nacks_total"
	// MetricWorkflowFailsTotal reports cumulative failures.
	MetricWorkflowFailsTotal = "levier_workflow_fails_total"
	// MetricWorkflowDeadTotal reports cumulative dead-lettered tasks.
	MetricWorkflowDeadTotal = "levier_workflow_dead_total"
	// MetricWorkflowReapedTotal reports cumulative expired leases reclaimed.
	MetricWorkflowReapedTotal = "levier_workflow_reaped_total"
	// MetricWorkflowLeaseLostTotal reports cumulative lost leases.
	MetricWorkflowLeaseLostTotal = "levier_workflow_lease_lost_total"
	// MetricWorkflowPanicsTotal reports cumulative handler panics.
	MetricWorkflowPanicsTotal = "levier_workflow_panics_total"
	// MetricWorkflowClaimDuration reports average claim duration in seconds.
	MetricWorkflowClaimDuration = "levier_workflow_claim_duration_seconds"
	// MetricWorkflowHandlerDuration reports average handler duration in seconds.
	MetricWorkflowHandlerDuration = "levier_workflow_handler_duration_seconds"
	// MetricDaemonCapacity reports daemon capacity by kind (used/total).
	MetricDaemonCapacity = "levier_daemon_capacity"
	// MetricStaleDaemonEventsTotal counts dropped stale-epoch events.
	MetricStaleDaemonEventsTotal = "levier_stale_daemon_events_total"
	// MetricEventGapsTotal counts event sequence gaps.
	MetricEventGapsTotal = "levier_event_gaps_total"
	// MetricEphemeralEventsTotal counts skipped ephemeral events.
	MetricEphemeralEventsTotal = "levier_ephemeral_events_total"
	// MetricTokenUsageTotal counts token usage from USAGE events.
	MetricTokenUsageTotal = "levier_token_usage_total"
)
