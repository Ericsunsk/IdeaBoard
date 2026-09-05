package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"
)

// DefaultShutdownBudget defines the standard time window for draining in-flight work.
const DefaultShutdownBudget = 10 * time.Second

// Coordinator enforces runtime state machine progression and graceful shutdown budgets.
type Coordinator struct {
	stateMachine   *domain.StateMachine
	shutdownBudget time.Duration
}

// NewCoordinator initializes a lifecycle coordinator.
func NewCoordinator(budget time.Duration) *Coordinator {
	if budget <= 0 {
		budget = DefaultShutdownBudget
	}
	return &Coordinator{
		stateMachine:   domain.NewStateMachine(),
		shutdownBudget: budget,
	}
}

// State returns the current lifecycle state.
func (c *Coordinator) State() domain.State {
	return c.stateMachine.Current()
}

// CanAcceptTasks returns whether runtime is ready for new task execution.
func (c *Coordinator) CanAcceptTasks() bool {
	return c.stateMachine.CanAcceptTasks()
}

// BeginInitialization transitions from created to initializing.
func (c *Coordinator) BeginInitialization() error {
	return c.stateMachine.TransitionTo(domain.StateInitializing)
}

// BeginRecovery transitions from initializing to recovering.
func (c *Coordinator) BeginRecovery() error {
	return c.stateMachine.TransitionTo(domain.StateRecovering)
}

// MarkReady verifies readiness of required components and transitions to ready.
func (c *Coordinator) MarkReady(components []domain.ComponentStatus) error {
	if err := domain.EvaluateReadiness(components); err != nil {
		return fmt.Errorf("readiness evaluation failed: %w", err)
	}
	return c.stateMachine.TransitionTo(domain.StateReady)
}

// DrainAndShutdown stops runners within the configured shutdown budget.
func (c *Coordinator) DrainAndShutdown(ctx context.Context, runners []ports.Runner) error {
	if err := c.stateMachine.TransitionTo(domain.StateDraining); err != nil {
		slog.Warn("failed transition to draining", "err", err)
	}

	budgetCtx, cancel := context.WithTimeout(ctx, c.shutdownBudget)
	defer cancel()

	var errs []error
	for i := len(runners) - 1; i >= 0; i-- {
		runner := runners[i]
		slog.Info("stopping runner", "name", runner.Name(), "kind", runner.Kind())
		if err := runner.Stop(budgetCtx); err != nil {
			errs = append(errs, fmt.Errorf("stop runner %s: %w", runner.Name(), err))
		}
	}

	return errors.Join(errs...)
}

// StopResources transitions state to stopped and closes owned resources.
func (c *Coordinator) StopResources(closer ports.ResourceCloser) error {
	defer func() {
		_ = c.stateMachine.TransitionTo(domain.StateStopped)
	}()

	if closer == nil {
		return nil
	}
	return closer.Close()
}
