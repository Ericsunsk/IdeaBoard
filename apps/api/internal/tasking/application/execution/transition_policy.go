package execution

import (
	"context"
	"errors"
	"fmt"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	"strings"

	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
)

type taskTransition string

const (
	taskTransitionComplete     taskTransition = "complete"
	taskTransitionFailPending  taskTransition = "fail_pending"
	taskTransitionFailRunning  taskTransition = "fail_running"
	taskTransitionReset        taskTransition = "reset"
	taskTransitionRequeue      taskTransition = "requeue"
	taskTransitionFinalFail    taskTransition = "final_fail"
	taskTransitionRecoveryFail taskTransition = "recovery_fail"
	taskTransitionCancel       taskTransition = "cancel"
)

type taskTransitionRule struct {
	from          []string
	target        string
	idempotent    []string
	requiresFence bool
	// Lease-owned finalization that sees a terminal task is treated as stale work, not success.
	terminalConflict bool
}

var taskTransitionRules = map[taskTransition]taskTransitionRule{
	taskTransitionComplete: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusCompleted,
		requiresFence:    true,
		terminalConflict: true,
	},
	taskTransitionFailPending: {
		from:       []string{tasks.StatusPending},
		target:     tasks.StatusFailed,
		idempotent: []string{tasks.StatusFailed},
	},
	taskTransitionFailRunning: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusFailed,
		requiresFence:    true,
		terminalConflict: true,
	},
	taskTransitionReset: {
		from:          []string{tasks.StatusRunning},
		target:        tasks.StatusPending,
		idempotent:    []string{tasks.StatusPending},
		requiresFence: true,
	},
	taskTransitionRequeue: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusPending,
		idempotent:       []string{tasks.StatusPending},
		requiresFence:    true,
		terminalConflict: true,
	},
	taskTransitionFinalFail: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusFailed,
		idempotent:       []string{tasks.StatusPending},
		requiresFence:    true,
		terminalConflict: true,
	},
	taskTransitionRecoveryFail: {
		from:       []string{tasks.StatusPending, tasks.StatusRunning},
		target:     tasks.StatusFailed,
		idempotent: []string{tasks.StatusFailed},
	},
	taskTransitionCancel: {
		from:       []string{tasks.StatusPending, tasks.StatusRunning},
		target:     tasks.StatusCancelled,
		idempotent: []string{tasks.StatusCancelled, tasks.StatusCancelledWithResults},
	},
}

type taskTransitionCurrentResult struct {
	ready      bool
	idempotent bool
}

func requireTaskTransitionToken(transition taskTransition, token string) error {
	rule := taskTransitionRules[transition]
	if rule.requiresFence && strings.TrimSpace(token) == "" {
		return ErrTaskStateConflict
	}
	return nil
}

func normalizeTaskTransitionWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, runtimecontracts.ErrTaskLeaseStateConflict) {
		return fmt.Errorf("%w: %w", ErrTaskStateConflict, err)
	}
	return err
}

func (s *Service) resolveTaskTransitionResult(ctx context.Context, transition taskTransition, taskID string, rows int64) (bool, error) {
	if rows > 0 {
		return true, nil
	}
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		if database.IsNotFound(err) {
			return false, ErrTaskNotFound
		}
		return false, err
	}
	result, err := evaluateTaskTransitionCurrent(transition, task)
	if err != nil {
		return false, err
	}
	if result.ready {
		return false, ErrTaskStateConflict
	}
	return !result.idempotent, nil
}

// evaluateTaskTransitionCurrent classifies a zero-row write: the task still
// carries a fence token (another worker owns it → conflict), sits in an
// idempotent state, or is gone/terminal.
func evaluateTaskTransitionCurrent(transition taskTransition, task *tasks.Task) (taskTransitionCurrentResult, error) {
	rule, ok := taskTransitionRules[transition]
	if !ok {
		return taskTransitionCurrentResult{}, ErrTaskStateConflict
	}
	if task == nil {
		return taskTransitionCurrentResult{}, ErrTaskNotFound
	}
	if rule.hasStatus(rule.idempotent, task.Status) {
		return taskTransitionCurrentResult{idempotent: true}, nil
	}
	if rule.requiresFence && strings.TrimSpace(task.LeaseToken) != "" {
		return taskTransitionCurrentResult{}, ErrTaskStateConflict
	}
	if tasks.IsTerminalStatus(task.Status) {
		if rule.terminalConflict {
			return taskTransitionCurrentResult{}, ErrTaskStateConflict
		}
		return taskTransitionCurrentResult{}, ErrTaskTerminal
	}
	if !rule.hasStatus(rule.from, task.Status) {
		return taskTransitionCurrentResult{}, ErrTaskStateConflict
	}
	return taskTransitionCurrentResult{ready: true}, nil
}

func (r taskTransitionRule) hasStatus(statuses []string, status string) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}
