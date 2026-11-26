// Command i3daemon is a scripted Runtime daemon used by the I3 multi-replica
// fault matrix. It is intentionally small: connect, acknowledge TaskAssign,
// optionally invoke one App-local tool, then emit a terminal Run event.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/testing/testutil/fakedaemon"
)

func main() {
	var (
		gateway     = flag.String("gateway", "127.0.0.1:19090", "Runtime Gateway gRPC address")
		token       = flag.String("token", "change-me", "daemon auth token")
		daemonID    = flag.String("daemon-id", "i3-daemon", "daemon id")
		runtimeName = flag.String("runtime", "i3daemon", "runtime name")
		tool        = flag.String("tool", "", "optional local tool name")
		args        = flag.String("args", "{}", "tool arguments JSON")
		delay       = flag.Duration("delay", 0, "delay before emitting completion")
		repeat      = flag.Int("repeat-invocation", 1, "send the same invocation N times")
	)
	flag.Parse()

	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = *daemonID
	cfg.AuthToken = *token
	cfg.RuntimeName = *runtimeName
	cfg.MaxConcurrentRuns = 4
	cfg.Labels = map[string]string{"pool": "i3"}
	cfg.HeartbeatInterval = time.Second
	cfg.OnAssign = func(ctx context.Context, env *runtimev1.Envelope, daemon *fakedaemon.Daemon) error {
		assign := env.GetTaskAssign()
		if assign == nil {
			return nil
		}
		go runAssignment(ctx, daemon, assign, env, *tool, *args, *delay, *repeat)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemon, err := fakedaemon.Dial(ctx, *gateway, cfg)
	if err != nil {
		log.Printf("i3daemon: connect %s: %v", *gateway, err)
		os.Exit(1)
	}
	defer daemon.Close()
	log.Printf("i3daemon: connected daemon=%s runtime=%s gateway=%s", *daemonID, *runtimeName, *gateway)
	<-ctx.Done()
}

func runAssignment(
	ctx context.Context,
	daemon *fakedaemon.Daemon,
	assign *runtimev1.TaskAssign,
	env *runtimev1.Envelope,
	tool, args string,
	delay time.Duration,
	repeat int,
) {
	runID := assign.GetRunId()
	attempt := env.GetAttempt()
	leaseEpoch := env.GetLeaseEpoch()
	log.Printf("i3daemon: assigned run=%s attempt=%d epoch=%d", runID, attempt, leaseEpoch)
	_ = daemon.SendEvent(runID, attempt, leaseEpoch, runtimev1.EventType_RUN_STATUS,
		map[string]any{"status": "running"})
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
	if strings.TrimSpace(tool) != "" {
		invocationID := id.New()
		timeout := int64(10000)
		var last *runtimev1.ToolCallResult
		for i := 0; i < repeat; i++ {
			result, err := daemon.RequestToolWithInvocation(
				ctx, runID, attempt, leaseEpoch, invocationID, tool, args, timeout,
			)
			if err != nil {
				log.Printf("i3daemon: tool %s failed run=%s: %v", tool, runID, err)
				_ = daemon.SendEvent(runID, attempt, leaseEpoch, runtimev1.EventType_RUN_STATUS,
					map[string]any{"status": "failed", "error": err.Error()})
				return
			}
			last = result
		}
		if last == nil || !last.GetOk() {
			log.Printf("i3daemon: tool %s rejected run=%s: %+v", tool, runID, last)
			_ = daemon.SendEvent(runID, attempt, leaseEpoch, runtimev1.EventType_RUN_STATUS,
				map[string]any{"status": "failed"})
			return
		}
		log.Printf("i3daemon: tool %s ok run=%s result=%s", tool, runID, last.GetResultJson())
	}
	_ = daemon.SendEvent(runID, attempt, leaseEpoch, runtimev1.EventType_MESSAGE_COMPLETED,
		map[string]any{"role": "assistant", "content": "i3daemon completed"})
	_ = daemon.SendEvent(runID, attempt, leaseEpoch, runtimev1.EventType_RUN_STATUS,
		map[string]any{"status": "completed"})
}
