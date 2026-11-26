// Levier - PostgreSQL-backed connection registry and leases.
//
// Copyright 2026 The Levier Authors.
package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/platform/id"
)

const (
	defaultLeaseTTL = 30 * time.Second
)

const gatewayColumns = `
	gateway_id, epoch, roles, status, version, address, internal_address, labels_json,
	started_at, last_seen_at, lease_expires_at`

const runtimeColumns = `
	daemon_id, gateway_id, connection_epoch, runtime_name, runtime_type, status,
	version, max_concurrent_runs, running_count, reserved, labels_json,
	capabilities_json, running_runs_json, connected_at, last_heartbeat_at,
	lease_expires_at`

const appConnectionColumns = `
	connection_id, app_id, tenant_id, gateway_id, connection_epoch,
	protocol_version, capabilities_json, status, connected_at,
	last_heartbeat_at, lease_expires_at`

const executionLeaseColumns = `
	app_id, connection_id, gateway_id, lease_epoch, expires_at, updated_at`

const notificationColumns = `
	notification_id, channel, dedupe_key, payload_json, attempts,
	next_attempt_at, claimed_by, claimed_until, created_at, published_at`

// Store is the PostgreSQL connection registry used by I3 replicas.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a registry store.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// RegisterGateway creates or takes over a gateway instance lease.
func (s *Store) RegisterGateway(ctx context.Context, reg GatewayRegistration, ttl time.Duration) (GatewayInstance, error) {
	if err := s.ready(); err != nil {
		return GatewayInstance{}, err
	}
	if reg.GatewayID == "" || len(reg.Roles) == 0 {
		return GatewayInstance{}, fmt.Errorf("%w: gateway_id and roles are required", ErrInvalidRegistration)
	}
	labels, err := marshalMap(reg.Labels)
	if err != nil {
		return GatewayInstance{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	gateway, err := scanGateway(s.pool.QueryRow(ctx, `
		INSERT INTO gateway_instances (
			gateway_id, epoch, roles, status, version, address, internal_address, labels_json,
			started_at, last_seen_at, lease_expires_at, updated_at
		) VALUES ($1, 1, $2, 'online', $3, $4, $5, $6, now(), now(), $7, now())
		ON CONFLICT (gateway_id) DO UPDATE SET
			epoch = gateway_instances.epoch + 1,
			roles = EXCLUDED.roles,
			status = 'online',
			version = EXCLUDED.version,
			address = EXCLUDED.address,
			internal_address = EXCLUDED.internal_address,
			labels_json = EXCLUDED.labels_json,
			started_at = EXCLUDED.started_at,
			last_seen_at = now(),
			lease_expires_at = EXCLUDED.lease_expires_at,
			updated_at = now()
		RETURNING `+gatewayColumns,
		reg.GatewayID, reg.Roles, reg.Version, reg.Address, reg.InternalAddress, labels, expires,
	))
	if err != nil {
		return GatewayInstance{}, fmt.Errorf("register gateway: %w", err)
	}
	return gateway, nil
}

// HeartbeatGateway renews a gateway instance lease.
func (s *Store) HeartbeatGateway(ctx context.Context, gatewayID string, epoch int64, ttl time.Duration) (GatewayInstance, error) {
	if err := s.ready(); err != nil {
		return GatewayInstance{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	gateway, err := scanGateway(s.pool.QueryRow(ctx, `
		UPDATE gateway_instances
		SET last_seen_at = now(),
		    lease_expires_at = $3,
		    updated_at = now()
		WHERE gateway_id = $1
		  AND epoch = $2
		  AND status = 'online'
		RETURNING `+gatewayColumns,
		gatewayID, epoch, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return GatewayInstance{}, ErrLeaseLost
	}
	if err != nil {
		return GatewayInstance{}, fmt.Errorf("heartbeat gateway: %w", err)
	}
	return gateway, nil
}

// MarkGatewayOffline releases a gateway instance lease.
func (s *Store) MarkGatewayOffline(ctx context.Context, gatewayID string, epoch int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE gateway_instances
		SET status = 'offline',
		    lease_expires_at = now(),
		    updated_at = now()
		WHERE gateway_id = $1
		  AND epoch = $2
		  AND status = 'online'
	`, gatewayID, epoch)
	if err != nil {
		return fmt.Errorf("mark gateway offline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

// GetGateway returns one gateway instance.
func (s *Store) GetGateway(ctx context.Context, gatewayID string) (GatewayInstance, error) {
	if err := s.ready(); err != nil {
		return GatewayInstance{}, err
	}
	gateway, err := scanGateway(s.pool.QueryRow(ctx,
		"SELECT "+gatewayColumns+" FROM gateway_instances WHERE gateway_id = $1",
		gatewayID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return GatewayInstance{}, ErrConnectionNotFound
	}
	if err != nil {
		return GatewayInstance{}, fmt.Errorf("get gateway: %w", err)
	}
	return gateway, nil
}

// ListGatewaysByRole returns live gateways advertising one role.
func (s *Store) ListGatewaysByRole(ctx context.Context, role string) ([]GatewayInstance, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+gatewayColumns+`
		FROM gateway_instances
		WHERE status = 'online'
		  AND lease_expires_at > now()
		  AND $1 = ANY(roles)
		ORDER BY gateway_id
	`, role)
	if err != nil {
		return nil, fmt.Errorf("list gateways by role: %w", err)
	}
	defer rows.Close()
	result := make([]GatewayInstance, 0)
	for rows.Next() {
		gateway, err := scanGateway(rows)
		if err != nil {
			return nil, fmt.Errorf("list gateways by role: scan: %w", err)
		}
		result = append(result, gateway)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gateways by role: rows: %w", err)
	}
	return result, nil
}

// RegisterRuntime creates or takes over a Runtime connection lease.
func (s *Store) RegisterRuntime(ctx context.Context, reg RuntimeRegistration, ttl time.Duration) (RuntimeConnection, error) {
	if err := s.ready(); err != nil {
		return RuntimeConnection{}, err
	}
	if reg.DaemonID == "" || reg.GatewayID == "" || reg.RuntimeName == "" || reg.RuntimeType == "" {
		return RuntimeConnection{}, fmt.Errorf("%w: daemon, gateway, runtime_name, and runtime_type are required", ErrInvalidRegistration)
	}
	if reg.MaxConcurrentRuns <= 0 {
		reg.MaxConcurrentRuns = 1
	}
	labels, err := marshalMap(reg.Labels)
	if err != nil {
		return RuntimeConnection{}, err
	}
	capabilities, err := marshalMap(reg.Capabilities)
	if err != nil {
		return RuntimeConnection{}, err
	}
	runningRuns, err := marshalRunningRuns(reg.RunningRuns)
	if err != nil {
		return RuntimeConnection{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	connection, err := scanRuntimeConnection(s.pool.QueryRow(ctx, `
		INSERT INTO runtime_connections (
			daemon_id, gateway_id, connection_epoch, runtime_name, runtime_type,
			status, version, max_concurrent_runs, running_count, reserved,
			labels_json, capabilities_json, running_runs_json, connected_at,
			last_heartbeat_at, lease_expires_at, updated_at
		) VALUES (
			$1, $2, 1, $3, $4, 'online', $5, $6, $7, 0,
			$8, $9, $10, now(), now(), $11, now()
		)
		ON CONFLICT (daemon_id) DO UPDATE SET
			gateway_id = EXCLUDED.gateway_id,
			connection_epoch = runtime_connections.connection_epoch + 1,
			runtime_name = EXCLUDED.runtime_name,
			runtime_type = EXCLUDED.runtime_type,
			status = 'online',
			version = EXCLUDED.version,
			max_concurrent_runs = EXCLUDED.max_concurrent_runs,
			running_count = EXCLUDED.running_count,
			labels_json = EXCLUDED.labels_json,
			capabilities_json = EXCLUDED.capabilities_json,
			running_runs_json = EXCLUDED.running_runs_json,
			connected_at = now(),
			last_heartbeat_at = now(),
			lease_expires_at = EXCLUDED.lease_expires_at,
			updated_at = now()
		WHERE runtime_connections.status = 'offline'
		   OR runtime_connections.lease_expires_at < now()
		   OR runtime_connections.gateway_id = EXCLUDED.gateway_id
		RETURNING `+runtimeColumns,
		reg.DaemonID, reg.GatewayID, reg.RuntimeName, reg.RuntimeType,
		reg.Version, reg.MaxConcurrentRuns, len(reg.RunningRuns),
		labels, capabilities, runningRuns, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeConnection{}, ErrLeaseHeld
	}
	if err != nil {
		return RuntimeConnection{}, fmt.Errorf("register runtime: %w", err)
	}
	return connection, nil
}

// HeartbeatRuntime renews a Runtime connection lease and capacity snapshot.
func (s *Store) HeartbeatRuntime(
	ctx context.Context,
	daemonID, gatewayID string,
	epoch int64,
	runningCount, reserved int,
	runningRuns []RuntimeRunRef,
	ttl time.Duration,
) (RuntimeConnection, error) {
	if err := s.ready(); err != nil {
		return RuntimeConnection{}, err
	}
	encodedRuns, err := marshalRunningRuns(runningRuns)
	if err != nil {
		return RuntimeConnection{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	connection, err := scanRuntimeConnection(s.pool.QueryRow(ctx, `
		UPDATE runtime_connections
		SET running_count = $4,
		    reserved = $5,
		    running_runs_json = $6,
		    last_heartbeat_at = now(),
		    lease_expires_at = $7,
		    updated_at = now()
		WHERE daemon_id = $1
		  AND gateway_id = $2
		  AND connection_epoch = $3
		  AND status = 'online'
		RETURNING `+runtimeColumns,
		daemonID, gatewayID, epoch, runningCount, reserved, encodedRuns, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeConnection{}, ErrLeaseLost
	}
	if err != nil {
		return RuntimeConnection{}, fmt.Errorf("heartbeat runtime: %w", err)
	}
	return connection, nil
}

// MarkRuntimeOffline releases a Runtime connection lease.
func (s *Store) MarkRuntimeOffline(ctx context.Context, daemonID string, epoch int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE runtime_connections
		SET status = 'offline',
		    lease_expires_at = now(),
		    updated_at = now()
		WHERE daemon_id = $1
		  AND connection_epoch = $2
		  AND status = 'online'
	`, daemonID, epoch)
	if err != nil {
		return fmt.Errorf("mark runtime offline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

// GetRuntime returns one Runtime connection.
func (s *Store) GetRuntime(ctx context.Context, daemonID string) (RuntimeConnection, error) {
	if err := s.ready(); err != nil {
		return RuntimeConnection{}, err
	}
	connection, err := scanRuntimeConnection(s.pool.QueryRow(ctx,
		"SELECT "+runtimeColumns+" FROM runtime_connections WHERE daemon_id = $1",
		daemonID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeConnection{}, ErrConnectionNotFound
	}
	if err != nil {
		return RuntimeConnection{}, fmt.Errorf("get runtime: %w", err)
	}
	return connection, nil
}

// ListOnlineRuntimes returns live Runtime connections.
func (s *Store) ListOnlineRuntimes(ctx context.Context) ([]RuntimeConnection, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runtimeColumns+`
		FROM runtime_connections
		WHERE status = 'online' AND lease_expires_at > now()
		ORDER BY daemon_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}
	return scanRuntimeConnections(rows)
}

// RegisterAppConnection creates or takes over an App connection lease.
func (s *Store) RegisterAppConnection(ctx context.Context, reg AppConnectionRegistration, ttl time.Duration) (AppConnection, error) {
	if err := s.ready(); err != nil {
		return AppConnection{}, err
	}
	if reg.AppID == "" || reg.TenantID == "" || reg.GatewayID == "" {
		return AppConnection{}, fmt.Errorf("%w: app, tenant, and gateway are required", ErrInvalidRegistration)
	}
	if reg.ConnectionID == "" {
		reg.ConnectionID = "conn_" + id.New()
	}
	if reg.ProtocolVersion <= 0 {
		reg.ProtocolVersion = 1
	}
	capabilities, err := marshalMap(reg.Capabilities)
	if err != nil {
		return AppConnection{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	connection, err := scanAppConnection(s.pool.QueryRow(ctx, `
		INSERT INTO app_connections (
			connection_id, app_id, tenant_id, gateway_id, connection_epoch,
			protocol_version, capabilities_json, status, connected_at,
			last_heartbeat_at, lease_expires_at, updated_at
		) VALUES ($1, $2, $3, $4, 1, $5, $6, 'online', now(), now(), $7, now())
		ON CONFLICT (connection_id) DO UPDATE SET
			gateway_id = EXCLUDED.gateway_id,
			connection_epoch = app_connections.connection_epoch + 1,
			protocol_version = EXCLUDED.protocol_version,
			capabilities_json = EXCLUDED.capabilities_json,
			status = 'online',
			connected_at = now(),
			last_heartbeat_at = now(),
			lease_expires_at = EXCLUDED.lease_expires_at,
			updated_at = now()
		WHERE app_connections.app_id = EXCLUDED.app_id
		  AND app_connections.tenant_id = EXCLUDED.tenant_id
		RETURNING `+appConnectionColumns,
		reg.ConnectionID, reg.AppID, reg.TenantID, reg.GatewayID,
		reg.ProtocolVersion, capabilities, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return AppConnection{}, ErrLeaseLost
	}
	if err != nil {
		return AppConnection{}, fmt.Errorf("register app connection: %w", err)
	}
	return connection, nil
}

// HeartbeatAppConnection renews an App connection lease.
func (s *Store) HeartbeatAppConnection(ctx context.Context, connectionID, gatewayID string, epoch int64, ttl time.Duration) (AppConnection, error) {
	if err := s.ready(); err != nil {
		return AppConnection{}, err
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	connection, err := scanAppConnection(s.pool.QueryRow(ctx, `
		UPDATE app_connections
		SET last_heartbeat_at = now(),
		    lease_expires_at = $4,
		    updated_at = now()
		WHERE connection_id = $1
		  AND gateway_id = $2
		  AND connection_epoch = $3
		  AND status = 'online'
		RETURNING `+appConnectionColumns,
		connectionID, gatewayID, epoch, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return AppConnection{}, ErrLeaseLost
	}
	if err != nil {
		return AppConnection{}, fmt.Errorf("heartbeat app connection: %w", err)
	}
	return connection, nil
}

// UpdateAppConnectionMetadata records the negotiated App Bridge version and
// capabilities after the WebSocket hello is processed.
func (s *Store) UpdateAppConnectionMetadata(
	ctx context.Context,
	connectionID string,
	protocolVersion int,
	capabilities map[string]string,
) error {
	if err := s.ready(); err != nil {
		return err
	}
	if connectionID == "" || protocolVersion <= 0 {
		return fmt.Errorf("%w: connection_id and protocol_version are required", ErrInvalidRegistration)
	}
	encoded, err := marshalMap(capabilities)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE app_connections
		SET protocol_version = $2,
		    capabilities_json = $3,
		    updated_at = now()
		WHERE connection_id = $1
		  AND status = 'online'
	`, connectionID, protocolVersion, encoded)
	if err != nil {
		return fmt.Errorf("update app connection metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

// MarkAppConnectionOffline releases an App connection lease.
func (s *Store) MarkAppConnectionOffline(ctx context.Context, connectionID string, epoch int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE app_connections
		SET status = 'offline',
		    lease_expires_at = now(),
		    updated_at = now()
		WHERE connection_id = $1
		  AND connection_epoch = $2
		  AND status = 'online'
	`, connectionID, epoch)
	if err != nil {
		return fmt.Errorf("mark app connection offline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

// AcquireCapacityLease reserves one Runtime capacity slot atomically.
func (s *Store) AcquireCapacityLease(
	ctx context.Context,
	daemonID, reservationID, gatewayID string,
	ttl time.Duration,
) (CapacityLease, error) {
	if err := s.ready(); err != nil {
		return CapacityLease{}, err
	}
	if daemonID == "" || reservationID == "" || gatewayID == "" {
		return CapacityLease{}, fmt.Errorf("%w: daemon, reservation, and gateway are required", ErrInvalidRegistration)
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CapacityLease{}, fmt.Errorf("acquire capacity: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", daemonID); err != nil {
		return CapacityLease{}, fmt.Errorf("acquire capacity: lock: %w", err)
	}
	var maxRuns int
	if err := tx.QueryRow(ctx, `
		SELECT max_concurrent_runs
		FROM runtime_connections
		WHERE daemon_id = $1
		  AND gateway_id = $2
		  AND status = 'online'
		  AND lease_expires_at > now()
	`, daemonID, gatewayID).Scan(&maxRuns); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CapacityLease{}, ErrLeaseLost
		}
		return CapacityLease{}, fmt.Errorf("acquire capacity: runtime: %w", err)
	}
	if maxRuns <= 0 {
		maxRuns = 1
	}
	var existing bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM runtime_capacity_leases
			WHERE daemon_id = $1 AND reservation_id = $2 AND expires_at > now()
		)
	`, daemonID, reservationID).Scan(&existing); err != nil {
		return CapacityLease{}, fmt.Errorf("acquire capacity: existing: %w", err)
	}
	if !existing {
		var active int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM runtime_capacity_leases
			WHERE daemon_id = $1 AND expires_at > now()
		`, daemonID).Scan(&active); err != nil {
			return CapacityLease{}, fmt.Errorf("acquire capacity: usage: %w", err)
		}
		if active >= maxRuns {
			return CapacityLease{}, ErrCapacityExceeded
		}
	}
	lease, err := scanCapacityLease(tx.QueryRow(ctx, `
		INSERT INTO runtime_capacity_leases (
			daemon_id, reservation_id, gateway_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (daemon_id, reservation_id) DO UPDATE SET
			gateway_id = EXCLUDED.gateway_id,
			expires_at = EXCLUDED.expires_at,
			updated_at = now()
		RETURNING daemon_id, reservation_id, gateway_id, expires_at, created_at, updated_at
	`, daemonID, reservationID, gatewayID, expires))
	if err != nil {
		return CapacityLease{}, fmt.Errorf("acquire capacity: upsert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CapacityLease{}, fmt.Errorf("acquire capacity: commit: %w", err)
	}
	return lease, nil
}

// ReleaseCapacityLease releases one reservation.
func (s *Store) ReleaseCapacityLease(ctx context.Context, daemonID, reservationID string) (bool, error) {
	if err := s.ready(); err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM runtime_capacity_leases
		WHERE daemon_id = $1 AND reservation_id = $2
	`, daemonID, reservationID)
	if err != nil {
		return false, fmt.Errorf("release capacity: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GetAppConnection returns one App connection.
func (s *Store) GetAppConnection(ctx context.Context, connectionID string) (AppConnection, error) {
	if err := s.ready(); err != nil {
		return AppConnection{}, err
	}
	connection, err := scanAppConnection(s.pool.QueryRow(ctx,
		"SELECT "+appConnectionColumns+" FROM app_connections WHERE connection_id = $1",
		connectionID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return AppConnection{}, ErrConnectionNotFound
	}
	if err != nil {
		return AppConnection{}, fmt.Errorf("get app connection: %w", err)
	}
	return connection, nil
}

// ListAppConnections returns connections for one App and tenant.
func (s *Store) ListAppConnections(ctx context.Context, appID, tenantID string) ([]AppConnection, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+appConnectionColumns+`
		FROM app_connections
		WHERE app_id = $1 AND tenant_id = $2
		  AND status = 'online'
		  AND lease_expires_at > now()
		ORDER BY connected_at DESC, connection_id
	`, appID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list app connections: %w", err)
	}
	return scanAppConnections(rows)
}

// AcquireAppExecutionLease gives one live App connection the local-tool lease.
func (s *Store) AcquireAppExecutionLease(ctx context.Context, appID, connectionID, gatewayID string, ttl time.Duration) (AppExecutionLease, error) {
	if err := s.ready(); err != nil {
		return AppExecutionLease{}, err
	}
	connection, err := s.GetAppConnection(ctx, connectionID)
	if err != nil {
		return AppExecutionLease{}, err
	}
	if connection.AppID != appID || connection.GatewayID != gatewayID ||
		connection.Status != StatusOnline || !connection.LeaseExpiresAt.After(time.Now().UTC()) {
		return AppExecutionLease{}, ErrLeaseLost
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	lease, err := scanExecutionLease(s.pool.QueryRow(ctx, `
		INSERT INTO app_execution_leases (
			app_id, connection_id, gateway_id, lease_epoch, expires_at, updated_at
		) VALUES ($1, $2, $3, 1, $4, now())
		ON CONFLICT (app_id) DO UPDATE SET
			connection_id = EXCLUDED.connection_id,
			gateway_id = EXCLUDED.gateway_id,
			lease_epoch = app_execution_leases.lease_epoch + 1,
			expires_at = EXCLUDED.expires_at,
			updated_at = now()
		WHERE app_execution_leases.expires_at < now()
		   OR app_execution_leases.connection_id = EXCLUDED.connection_id
		RETURNING `+executionLeaseColumns,
		appID, connectionID, gatewayID, expires,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return AppExecutionLease{}, ErrLeaseHeld
	}
	if err != nil {
		return AppExecutionLease{}, fmt.Errorf("acquire app execution lease: %w", err)
	}
	return lease, nil
}

// ReleaseAppExecutionLease drops an execution lease if the owner still matches.
func (s *Store) ReleaseAppExecutionLease(ctx context.Context, appID, connectionID string, epoch int64) (bool, error) {
	if err := s.ready(); err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM app_execution_leases
		WHERE app_id = $1
		  AND connection_id = $2
		  AND lease_epoch = $3
	`, appID, connectionID, epoch)
	if err != nil {
		return false, fmt.Errorf("release app execution lease: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GetAppExecutionLease returns the current local-tool lease for an App.
func (s *Store) GetAppExecutionLease(ctx context.Context, appID string) (AppExecutionLease, error) {
	if err := s.ready(); err != nil {
		return AppExecutionLease{}, err
	}
	lease, err := scanExecutionLease(s.pool.QueryRow(ctx,
		"SELECT "+executionLeaseColumns+" FROM app_execution_leases WHERE app_id = $1",
		appID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return AppExecutionLease{}, ErrConnectionNotFound
	}
	if err != nil {
		return AppExecutionLease{}, fmt.Errorf("get app execution lease: %w", err)
	}
	return lease, nil
}

// EnqueueNotification stores one durable wakeup for another replica.
func (s *Store) EnqueueNotification(ctx context.Context, channel string, payload json.RawMessage) (Notification, error) {
	return s.EnqueueNotificationKey(ctx, channel, "", payload)
}

// EnqueueNotificationKey stores a deduplicated wakeup when dedupeKey is set.
func (s *Store) EnqueueNotificationKey(ctx context.Context, channel, dedupeKey string, payload json.RawMessage) (Notification, error) {
	if err := s.ready(); err != nil {
		return Notification{}, err
	}
	if channel == "" || !json.Valid(payload) {
		return Notification{}, fmt.Errorf("%w: channel and valid JSON payload are required", ErrInvalidRegistration)
	}
	notification, err := scanNotification(s.pool.QueryRow(ctx, `
		INSERT INTO gateway_notifications (channel, dedupe_key, payload_json, next_attempt_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (dedupe_key) WHERE dedupe_key <> '' AND published_at IS NULL
		DO NOTHING
		RETURNING `+notificationColumns,
		channel, dedupeKey, string(payload)))
	if errors.Is(err, pgx.ErrNoRows) && dedupeKey != "" {
		notification, err = scanNotification(s.pool.QueryRow(ctx,
			"SELECT "+notificationColumns+" FROM gateway_notifications WHERE dedupe_key = $1 AND published_at IS NULL",
			dedupeKey,
		))
	}
	if err != nil {
		return Notification{}, fmt.Errorf("enqueue notification: %w", err)
	}
	return notification, nil
}

// ListPendingNotifications returns unpublished notifications due now.
func (s *Store) ListPendingNotifications(ctx context.Context, channel string, limit int) ([]Notification, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+notificationColumns+`
		FROM gateway_notifications
		WHERE published_at IS NULL
		  AND next_attempt_at <= now()
		  AND (claimed_until IS NULL OR claimed_until < now())
		  AND ($1 = '' OR channel = $1)
		ORDER BY notification_id
		LIMIT $2
	`, channel, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	result := make([]Notification, 0)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("list notifications: scan: %w", err)
		}
		result = append(result, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notifications: rows: %w", err)
	}
	return result, nil
}

// ClaimNotifications atomically leases due notifications to one consumer.
func (s *Store) ClaimNotifications(ctx context.Context, channel, consumer string, limit int, ttl time.Duration) ([]Notification, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if consumer == "" {
		return nil, fmt.Errorf("%w: consumer is required", ErrInvalidRegistration)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	expires := time.Now().UTC().Add(normalizeTTL(ttl))
	rows, err := s.pool.Query(ctx, `
		WITH claimed AS (
			SELECT notification_id
			FROM gateway_notifications
			WHERE published_at IS NULL
			  AND next_attempt_at <= now()
			  AND (claimed_until IS NULL OR claimed_until < now())
			  AND ($1 = '' OR channel = $1)
			ORDER BY notification_id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE gateway_notifications n
		SET claimed_by = $3,
		    claimed_until = $4,
		    attempts = attempts + 1,
		    updated_at = now()
		FROM claimed
		WHERE n.notification_id = claimed.notification_id
		RETURNING n.notification_id, n.channel, n.dedupe_key, n.payload_json,
		          n.attempts, n.next_attempt_at, n.claimed_by, n.claimed_until,
		          n.created_at, n.published_at
	`, channel, limit, consumer, expires)
	if err != nil {
		return nil, fmt.Errorf("claim notifications: %w", err)
	}
	defer rows.Close()
	result := make([]Notification, 0)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("claim notifications: scan: %w", err)
		}
		result = append(result, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim notifications: rows: %w", err)
	}
	return result, nil
}

// MarkNotificationPublished marks a notification as delivered.
func (s *Store) MarkNotificationPublished(ctx context.Context, notificationID int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE gateway_notifications
		SET published_at = now(),
		    claimed_by = '',
		    claimed_until = NULL,
		    attempts = attempts + 1
		WHERE notification_id = $1
	`, notificationID)
	if err != nil {
		return fmt.Errorf("mark notification published: %w", err)
	}
	return nil
}

// RescheduleNotification delays a failed notification delivery.
func (s *Store) RescheduleNotification(ctx context.Context, notificationID int64, delay time.Duration) error {
	if err := s.ready(); err != nil {
		return err
	}
	if delay <= 0 {
		delay = time.Second
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE gateway_notifications
		SET attempts = attempts + 1,
		    claimed_by = '',
		    claimed_until = NULL,
		    next_attempt_at = now() + $2
		WHERE notification_id = $1
	`, notificationID, delay)
	if err != nil {
		return fmt.Errorf("reschedule notification: %w", err)
	}
	return nil
}

// ReapExpired marks expired connections offline and removes stale execution leases.
func (s *Store) ReapExpired(ctx context.Context, now time.Time) (ReapResult, error) {
	if err := s.ready(); err != nil {
		return ReapResult{}, err
	}
	var result ReapResult
	tag, err := s.pool.Exec(ctx, `
		UPDATE gateway_instances
		SET status = 'offline', updated_at = now()
		WHERE status = 'online' AND lease_expires_at < $1
	`, now)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reap gateways: %w", err)
	}
	result.Gateways = tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `
		UPDATE runtime_connections
		SET status = 'offline', updated_at = now()
		WHERE status = 'online' AND lease_expires_at < $1
	`, now)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reap runtimes: %w", err)
	}
	result.Runtimes = tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `
		UPDATE app_connections
		SET status = 'offline', updated_at = now()
		WHERE status = 'online' AND lease_expires_at < $1
	`, now)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reap app connections: %w", err)
	}
	result.Apps = tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `
		DELETE FROM app_execution_leases WHERE expires_at < $1
	`, now)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reap app execution leases: %w", err)
	}
	result.Exec = tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `
		DELETE FROM runtime_capacity_leases WHERE expires_at < $1
	`, now)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reap capacity leases: %w", err)
	}
	result.Capacity = tag.RowsAffected()
	return result, nil
}

func (s *Store) ready() error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("cluster: postgres pool is nil")
	}
	return nil
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultLeaseTTL
	}
	return ttl
}

func marshalMap(value map[string]string) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: encode map: %v", ErrInvalidRegistration, err)
	}
	return encoded, nil
}

func marshalRunningRuns(value []RuntimeRunRef) ([]byte, error) {
	if value == nil {
		return []byte(`[]`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: encode running runs: %v", ErrInvalidRegistration, err)
	}
	return encoded, nil
}

func decodeMap(value []byte) (map[string]string, error) {
	if len(value) == 0 {
		return map[string]string{}, nil
	}
	result := make(map[string]string)
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeRunningRuns(value []byte) ([]RuntimeRunRef, error) {
	if len(value) == 0 {
		return []RuntimeRunRef{}, nil
	}
	var result []RuntimeRunRef
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func scanGateway(row pgx.Row) (GatewayInstance, error) {
	var gateway GatewayInstance
	var labels []byte
	if err := row.Scan(
		&gateway.GatewayID,
		&gateway.Epoch,
		&gateway.Roles,
		&gateway.Status,
		&gateway.Version,
		&gateway.Address,
		&gateway.InternalAddress,
		&labels,
		&gateway.StartedAt,
		&gateway.LastSeenAt,
		&gateway.LeaseExpiresAt,
	); err != nil {
		return GatewayInstance{}, err
	}
	decoded, err := decodeMap(labels)
	if err != nil {
		return GatewayInstance{}, fmt.Errorf("decode gateway labels: %w", err)
	}
	gateway.Labels = decoded
	return gateway, nil
}

func scanRuntimeConnection(row pgx.Row) (RuntimeConnection, error) {
	var connection RuntimeConnection
	var labels, capabilities, running []byte
	if err := row.Scan(
		&connection.DaemonID,
		&connection.GatewayID,
		&connection.ConnectionEpoch,
		&connection.RuntimeName,
		&connection.RuntimeType,
		&connection.Status,
		&connection.Version,
		&connection.MaxConcurrentRuns,
		&connection.RunningCount,
		&connection.Reserved,
		&labels,
		&capabilities,
		&running,
		&connection.ConnectedAt,
		&connection.LastHeartbeatAt,
		&connection.LeaseExpiresAt,
	); err != nil {
		return RuntimeConnection{}, err
	}
	connection.Labels, _ = decodeMap(labels)
	connection.Capabilities, _ = decodeMap(capabilities)
	connection.RunningRuns, _ = decodeRunningRuns(running)
	return connection, nil
}

func scanRuntimeConnections(rows pgx.Rows) ([]RuntimeConnection, error) {
	defer rows.Close()
	result := make([]RuntimeConnection, 0)
	for rows.Next() {
		connection, err := scanRuntimeConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan runtime: %w", err)
		}
		result = append(result, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtimes: %w", err)
	}
	return result, nil
}

func scanAppConnection(row pgx.Row) (AppConnection, error) {
	var connection AppConnection
	var capabilities []byte
	if err := row.Scan(
		&connection.ConnectionID,
		&connection.AppID,
		&connection.TenantID,
		&connection.GatewayID,
		&connection.ConnectionEpoch,
		&connection.ProtocolVersion,
		&capabilities,
		&connection.Status,
		&connection.ConnectedAt,
		&connection.LastHeartbeatAt,
		&connection.LeaseExpiresAt,
	); err != nil {
		return AppConnection{}, err
	}
	connection.Capabilities, _ = decodeMap(capabilities)
	return connection, nil
}

func scanAppConnections(rows pgx.Rows) ([]AppConnection, error) {
	defer rows.Close()
	result := make([]AppConnection, 0)
	for rows.Next() {
		connection, err := scanAppConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan app connection: %w", err)
		}
		result = append(result, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate app connections: %w", err)
	}
	return result, nil
}

func scanCapacityLease(row pgx.Row) (CapacityLease, error) {
	var lease CapacityLease
	if err := row.Scan(
		&lease.DaemonID,
		&lease.ReservationID,
		&lease.GatewayID,
		&lease.ExpiresAt,
		&lease.CreatedAt,
		&lease.UpdatedAt,
	); err != nil {
		return CapacityLease{}, err
	}
	return lease, nil
}

func scanExecutionLease(row pgx.Row) (AppExecutionLease, error) {
	var lease AppExecutionLease
	if err := row.Scan(
		&lease.AppID,
		&lease.ConnectionID,
		&lease.GatewayID,
		&lease.LeaseEpoch,
		&lease.ExpiresAt,
		&lease.UpdatedAt,
	); err != nil {
		return AppExecutionLease{}, err
	}
	return lease, nil
}

func scanNotification(row pgx.Row) (Notification, error) {
	var notification Notification
	var payload []byte
	var claimedUntil, published pgtype.Timestamptz
	if err := row.Scan(
		&notification.NotificationID,
		&notification.Channel,
		&notification.DedupeKey,
		&payload,
		&notification.Attempts,
		&notification.NextAttemptAt,
		&notification.ClaimedBy,
		&claimedUntil,
		&notification.CreatedAt,
		&published,
	); err != nil {
		return Notification{}, err
	}
	notification.Payload = json.RawMessage(payload)
	if claimedUntil.Valid {
		value := claimedUntil.Time
		notification.ClaimedUntil = &value
	}
	if published.Valid {
		value := published.Time
		notification.PublishedAt = &value
	}
	return notification, nil
}
