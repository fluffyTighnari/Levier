// Command levier runs the levier control plane.
//
// Levier — cmd/levier
//
// Loads configuration and starts the daemon-facing gRPC RuntimeService.
// Scheduling, HTTP, and control-plane wiring land in later V2 steps.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/control"
	"github.com/fluffyTighnari/levier/internal/db"
	"github.com/fluffyTighnari/levier/internal/gateway"
	"github.com/fluffyTighnari/levier/internal/gateway/executor"
	"github.com/fluffyTighnari/levier/internal/httpapi"
	"github.com/fluffyTighnari/levier/internal/observe"
	"github.com/fluffyTighnari/levier/internal/register"
	"github.com/fluffyTighnari/levier/internal/runstore"
	"github.com/fluffyTighnari/levier/internal/selfdesc"
	statuspkg "github.com/fluffyTighnari/levier/internal/status"
	"github.com/fluffyTighnari/levier/internal/version"
	"github.com/fluffyTighnari/levier/internal/workflow"
)

func runServer(args []string) {
	defaultConfig := os.Getenv("LEVIER_CONFIG")
	if defaultConfig == "" {
		defaultConfig = os.Getenv("MIDDLEWARE_CONFIG")
	}
	if defaultConfig == "" {
		defaultConfig = strings.TrimSpace(loadProfile().ControlConfig)
	}
	if defaultConfig == "" {
		defaultConfig = "levier.example.yaml"
	}
	flags := flag.NewFlagSet("levier", flag.ExitOnError)
	configPath := flags.String("config", defaultConfig, "path to YAML configuration")
	_ = flags.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "config", *configPath, "err", err)
		os.Exit(1)
	}

	logger, err := observe.NewLogger(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		slog.Error("initialize logger failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	startedAt := time.Now().UTC()

	pool, err := db.Open(ctx, db.Config{
		URL:            cfg.Database.URL,
		MaxConns:       cfg.Database.MaxConns,
		MinConns:       cfg.Database.MinConns,
		ConnectTimeout: cfg.Database.ConnectTimeout.Std(),
		HealthTimeout:  cfg.Database.HealthTimeout.Std(),
	})
	if err != nil {
		logger.Error("connect database failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	migrationCtx, cancelMigrations := context.WithTimeout(ctx, cfg.Database.HealthTimeout.Std())
	migrations, err := db.Status(migrationCtx, pool)
	cancelMigrations()
	if err != nil {
		logger.Error("check database migrations failed", "err", err)
		os.Exit(1)
	}
	if len(migrations.Pending) > 0 {
		logger.Error("database schema is behind",
			"current", migrations.Current,
			"pending", len(migrations.Pending),
			"fix", "run levier migrate up")
		os.Exit(1)
	}

	hub := connection.New(connection.Config{
		Authenticate: daemonAuthenticator(cfg.GRPC.DaemonTokens),
	})
	defer hub.Close()

	metrics := observe.NewRegistry()
	runs := runstore.New(pool)
	registry := register.New(pool)
	resolvers := []register.SecretResolver{register.EnvSecretResolver{}}
	if secretsPath := os.Getenv("LEVIER_SECRETS_FILE"); secretsPath != "" {
		resolver, err := register.LoadFileSecretResolver(secretsPath)
		if err != nil {
			logger.Error("load control-plane secrets failed", "err", err)
			os.Exit(1)
		}
		resolvers = append(resolvers, resolver)
	}
	managedPath := os.Getenv("LEVIER_SECRETS_WRITE_FILE")
	if managedPath == "" && os.Getenv("LEVIER_SECRETS_FILE") == "" {
		if dir, err := levierDataDir(); err == nil {
			managedPath = filepath.Join(dir, "secrets.json")
		}
	}
	var secretWriter register.SecretWriter
	if managedPath != "" {
		store, err := register.LoadMutableFileSecretStore(managedPath)
		if err != nil {
			logger.Error("load managed provider secrets failed", "err", err)
			os.Exit(1)
		}
		resolvers = append(resolvers, store)
		secretWriter = store
	}
	if len(resolvers) > 1 {
		registry = register.NewWithSecretsAndWriter(pool, register.ChainResolver{Resolvers: resolvers}, secretWriter)
	}
	queue := workflow.NewPostgres(pool)
	reaperDone := make(chan struct{})
	go func() {
		defer close(reaperDone)
		workflow.RunReaper(ctx, queue, cfg.Workflow.ReapInterval.Std(), cfg.Workflow.ReapLimit,
			func(result workflow.ReapResult) {
				if result.Err != nil {
					logger.Warn("workflow reaper failed", "err", result.Err)
					return
				}
				if result.Count == 0 {
					return
				}
				logger.Info("workflow reaped expired tasks", "count", result.Count)
				metrics.Add(observe.MetricWorkflowReapedTotal,
					"Cumulative expired leases reclaimed by the reaper.", nil, float64(result.Count))
			})
	}()
	appHub := connection.NewAppHub()
	defer appHub.Close()
	toolInvoker := gateway.NewInvoker(
		registry,
		appHub,
		gateway.NewExecutorRegistry(map[string]gateway.GlobalExecutor{
			"builtin:http":    executor.NewHTTP(),
			"builtin:webhook": executor.NewWebhook(),
		}),
		gateway.WithInvocationStore(gateway.NewPostgresInvocationStore(pool)),
		gateway.WithMetrics(metrics),
	)
	controller := control.New(runs, hub, control.Config{
		Logger:         logger,
		Publisher:      appHub,
		Scheduler:      control.NewHubScheduler(hub),
		Tools:          toolInvoker,
		Tasks:          queue,
		EventBuffer:    cfg.Workflow.EventBuffer,
		LocalTools:     appHub,
		ModelProviders: registry,
		Checkpoints:    runs,
		DaemonAvailable: func(daemonID string) bool {
			_, ok := hub.Get(daemonID)
			return ok
		},
		Metrics: metrics,
	}, registry)
	hub.SetOnEnvelope(func(daemonID string, env *runtimev1.Envelope) {
		if env.GetToolCallRequest() != nil {
			controller.DispatchToolCall(daemonID, env)
			return
		}
		controller.HandleEnvelope(daemonID, env)
	})
	hub.SetOnHello(controller.ReconcileDaemon)
	hub.SetOnOffline(controller.HandleDaemonOffline)
	controller.Start(ctx)
	defer controller.Close()

	if err := controller.RecoverQueued(ctx); err != nil {
		logger.Warn("recover queued runs failed", "err", err)
	}

	workers := workflow.NewPool(queue, controller, cfg.Workflow.Workers, workflow.WorkerConfig{
		Owner:        "control",
		LeaseTTL:     cfg.Workflow.LeaseTTL.Std(),
		PollInterval: cfg.Workflow.PollInterval.Std(),
	})
	workers.Start()
	defer workers.Stop()

	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		timer := time.NewTimer(cfg.Workflow.RecoveryDelay.Std())
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := controller.RecoverActive(ctx); err != nil {
			logger.Warn("recover active runs failed", "err", err)
		}
	}()

	metricsCollector := func(ctx context.Context) {
		stats, err := queue.Stats(ctx)
		if err != nil {
			logger.Warn("collect workflow metrics failed", "err", err)
			return
		}
		for _, taskType := range []workflow.TaskType{
			workflow.TaskDispatch, workflow.TaskRetry, workflow.TaskTimeout, workflow.TaskReconcile,
		} {
			for _, status := range []workflow.Status{
				workflow.StatusPending, workflow.StatusLeased, workflow.StatusDone,
				workflow.StatusDead, workflow.StatusCancelled,
			} {
				count := stats.ByTypeStatus[taskType][status]
				metrics.Set(observe.MetricWorkflowTasks, "Run tasks by type and status.", map[string]string{
					"type": string(taskType), "status": string(status),
				}, float64(count))
			}
			metrics.Set(observe.MetricWorkflowTaskAge, "Age of the oldest pending task by type.", map[string]string{
				"type": string(taskType),
			}, stats.OldestPendingByType[taskType].Seconds())
		}
		snapshot := workers.Metrics().Snapshot()
		metrics.SetCounter(observe.MetricWorkflowClaimsTotal, "Cumulative workflow task claims.", nil, float64(snapshot.Claims))
		metrics.SetCounter(observe.MetricWorkflowAcksTotal, "Cumulative workflow task acks.", nil, float64(snapshot.Acks))
		metrics.SetCounter(observe.MetricWorkflowNacksTotal, "Cumulative workflow task nacks.", nil, float64(snapshot.Nacks))
		metrics.SetCounter(observe.MetricWorkflowFailsTotal, "Cumulative workflow task failures.", nil, float64(snapshot.Fails))
		metrics.SetCounter(observe.MetricWorkflowDeadTotal, "Cumulative dead-lettered tasks.", nil, float64(snapshot.Dead))
		metrics.SetCounter(observe.MetricWorkflowLeaseLostTotal, "Cumulative lost leases.", nil, float64(snapshot.LeaseLost))
		metrics.SetCounter(observe.MetricWorkflowPanicsTotal, "Cumulative handler panics.", nil, float64(snapshot.Panics))
		metrics.Set(observe.MetricWorkflowClaimDuration, "Average workflow claim duration in seconds.", nil, snapshot.ClaimAverage.Seconds())
		metrics.Set(observe.MetricWorkflowHandlerDuration, "Average workflow handler duration in seconds.", nil, snapshot.HandlerAverage.Seconds())
		metrics.SetCounter(observe.MetricStaleDaemonEventsTotal, "Dropped stale-epoch events.", nil, float64(controller.StaleEvents()))
		metrics.SetCounter(observe.MetricEventGapsTotal, "Event sequence gaps.", nil, float64(controller.GapEvents()))
		metrics.SetCounter(observe.MetricEphemeralEventsTotal, "Skipped ephemeral events.", nil, float64(controller.EphemeralEvents()))
		for _, daemon := range hub.List() {
			used := daemon.RunningCount
			if daemon.Reserved > used {
				used = daemon.Reserved
			}
			total := daemon.MaxConcurrentRuns
			if total <= 0 {
				total = 1
			}
			labels := map[string]string{
				"daemon": daemon.DaemonID, "runtime": daemon.RuntimeType.String(),
			}
			metrics.Set(observe.MetricDaemonCapacity, "Daemon capacity by kind.", map[string]string{
				"daemon": labels["daemon"], "runtime": labels["runtime"], "kind": "used",
			}, float64(used))
			metrics.Set(observe.MetricDaemonCapacity, "Daemon capacity by kind.", map[string]string{
				"daemon": labels["daemon"], "runtime": labels["runtime"], "kind": "total",
			}, float64(total))
		}
	}

	listener, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		logger.Error("listen grpc failed", "addr", cfg.GRPC.Addr, "err", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(cfg.GRPC.MaxRecvMsgBytes),
		grpc.MaxSendMsgSize(cfg.GRPC.MaxSendMsgBytes),
	)
	hub.Register(grpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			logger.Error("grpc server stopped", "err", err)
		}
	}()

	statusService := statuspkg.New(statuspkg.Options{
		Hub:      hub,
		AppHub:   appHub,
		Runs:     runs,
		Workflow: queue,
		Health: func(ctx context.Context) error {
			return db.Health(ctx, pool, cfg.Database.HealthTimeout.Std())
		},
		Version:   version.Version,
		Commit:    version.Commit,
		StartedAt: startedAt,
	})

	api := httpapi.NewServer(runs, queue, httpapi.Config{
		BearerToken:          cfg.HTTP.BearerToken,
		AdminToken:           cfg.HTTP.AdminToken,
		AllowLegacyBearer:    cfg.HTTP.AllowLegacyBearer,
		WSTokenTTL:           cfg.HTTP.WSTokenTTL.Std(),
		DefaultRunTimeoutMS:  selfdesc.DefaultRunTimeoutMS,
		DefaultToolTimeoutMS: selfdesc.DefaultToolTimeoutMS,
		Meta: selfdesc.New(selfdesc.Options{
			Version:              version.Version,
			Commit:               version.Commit,
			Built:                version.Built,
			DefaultRunTimeoutMS:  selfdesc.DefaultRunTimeoutMS,
			DefaultToolTimeoutMS: selfdesc.DefaultToolTimeoutMS,
			MaxToolTimeoutMS:     selfdesc.DefaultMaxToolTimeoutMS,
			WSTokenTTL:           cfg.HTTP.WSTokenTTL.Std(),
		}),
		Status:  statusService,
		AppHub:  appHub,
		Daemons: hub,
		ReadinessCheck: func(ctx context.Context) error {
			return db.Health(ctx, pool, cfg.Database.HealthTimeout.Std())
		},
		Canceller:        controller,
		Resumer:          controller,
		Metrics:          metrics,
		MetricsCollector: metricsCollector,
		Logger:           logger,
	}, registry)
	httpListener, err := net.Listen("tcp", cfg.HTTP.Addr)
	if err != nil {
		logger.Error("listen http failed", "addr", cfg.HTTP.Addr, "err", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Handler:      api.Handler(),
		ReadTimeout:  cfg.HTTP.ReadTimeout.Std(),
		WriteTimeout: cfg.HTTP.WriteTimeout.Std(),
	}
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped", "err", err)
		}
	}()

	logger.Info("levier ready",
		"config", *configPath,
		"grpc_addr", cfg.GRPC.Addr,
		"http_addr", cfg.HTTP.Addr,
	)
	<-ctx.Done()

	shutdownStarted := time.Now()
	forcedShutdown := false
	logger.Info("levier stopping")
	workers.Stop()
	logger.Info("workflow workers stopped")
	controller.Close()
	logger.Info("control stopped")
	hub.Close()
	logger.Info("daemon hub stopped")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout.Std())
	_ = httpServer.Shutdown(shutdownCtx)
	cancel()
	_ = httpListener.Close()
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(cfg.GRPC.ShutdownTimeout.Std()):
		forcedShutdown = true
		logger.Warn("grpc graceful shutdown timed out; forcing stop")
		grpcServer.Stop()
	}
	_ = listener.Close()
	<-recoveryDone
	<-reaperDone
	logger.Info("levier stopped",
		"forced", forcedShutdown,
		"duration_ms", time.Since(shutdownStarted).Milliseconds(),
	)
}

func daemonAuthenticator(tokens map[string]string) connection.Authenticator {
	shared := tokens["*"]
	return func(daemonID, token string) bool {
		if expected, ok := tokens[daemonID]; ok {
			return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
		}
		// A shared runtime token lets the local SDK register a fresh runtime
		// without asking its author to invent a daemon id/token pair.
		if shared != "" {
			return subtle.ConstantTimeCompare([]byte(shared), []byte(token)) == 1
		}
		return false
	}
}
