package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/runtime/application/lifecycle"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

// StartupDependencies defines the components required to execute an ordered runtime startup.
type StartupDependencies struct {
	Recovery       ports.StartupRecoveryCoordinator
	Runners        []ports.Runner
	HealthCheckers []ports.HealthChecker
}

// ExecuteStartup runs the structured runtime startup sequence:
// 1. Begin initialization
// 2. Begin recovery and converge orphaned tasks
// 3. Launch runners in dependency order
// 4. Evaluate component health and mark ready
func ExecuteStartup(ctx context.Context, coord *lifecycle.Coordinator, deps StartupDependencies) error {
	if coord == nil {
		return fmt.Errorf("lifecycle coordinator is required")
	}
	if err := coord.BeginInitialization(); err != nil {
		return fmt.Errorf("begin initialization: %w", err)
	}

	if err := coord.BeginRecovery(); err != nil {
		return fmt.Errorf("begin recovery: %w", err)
	}

	if deps.Recovery != nil {
		if _, err := deps.Recovery.RecoverRunningTasks(ctx); err != nil {
			return fmt.Errorf("startup recovery failed: %w", err)
		}
	}

	started := make([]ports.Runner, 0, len(deps.Runners))
	for _, runner := range deps.Runners {
		slog.Info("starting runner", "name", runner.Name(), "kind", runner.Kind())
		if err := runner.Start(ctx); err != nil {
			// Startup is transactional from Runtime's perspective: if a later
			// component fails, stop everything that was already started so the
			// process cannot retain partially-owned listeners or workers.
			for i := len(started) - 1; i >= 0; i-- {
				if stopErr := started[i].Stop(ctx); stopErr != nil {
					slog.Warn("failed to rollback started runner", "name", started[i].Name(), "err", stopErr)
				}
			}
			return fmt.Errorf("start runner %s: %w", runner.Name(), err)
		}
		started = append(started, runner)
	}

	statuses := make([]domain.ComponentStatus, 0, len(deps.HealthCheckers))
	for _, checker := range deps.HealthCheckers {
		statuses = append(statuses, checker.CheckHealth(ctx))
	}

	if err := coord.MarkReady(statuses); err != nil {
		return fmt.Errorf("mark ready: %w", err)
	}
	return nil
}
