package taskruntime

import (
	"context"
	"errors"
	"fmt"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
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
	requiresLease bool
	// Lease-owned finalization that sees a terminal task is treated as stale work, not success.
	terminalConflict bool
}

var taskTransitionRules = map[taskTransition]taskTransitionRule{
	taskTransitionComplete: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusCompleted,
		requiresLease:    true,
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
		requiresLease:    true,
		terminalConflict: true,
	},
	taskTransitionReset: {
		from:          []string{tasks.StatusRunning},
		target:        tasks.StatusPending,
		idempotent:    []string{tasks.StatusPending},
		requiresLease: true,
	},
	taskTransitionRequeue: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusPending,
		idempotent:       []string{tasks.StatusPending},
		requiresLease:    true,
		terminalConflict: true,
	},
	taskTransitionFinalFail: {
		from:             []string{tasks.StatusRunning},
		target:           tasks.StatusFailed,
		idempotent:       []string{tasks.StatusPending},
		requiresLease:    true,
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
		idempotent: []string{tasks.StatusCancelled},
	},
}

type taskTransitionCurrentResult struct {
	ready      bool
	idempotent bool
}

func requireTaskTransitionLease(transition taskTransition, assignment taskqueue.TaskAssignment) error {
	rule := taskTransitionRules[transition]
	if rule.requiresLease && !assignment.HasLeaseIdentity() {
		return ErrTaskStateConflict
	}
	return nil
}

func normalizeTaskTransitionWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, taskstore.ErrTaskLeaseStateConflict) {
		return fmt.Errorf("%w: %v", ErrTaskStateConflict, err)
	}
	return err
}

func (s *Service) resolveTaskTransitionResult(ctx context.Context, transition taskTransition, taskID string, assignment taskqueue.TaskAssignment, rows int64) (bool, error) {
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
	result, err := evaluateTaskTransitionCurrent(transition, task, assignment)
	if err != nil {
		return false, err
	}
	if result.ready {
		return false, ErrTaskStateConflict
	}
	return !result.idempotent, nil
}

func evaluateTaskTransitionCurrent(transition taskTransition, task *tasks.Task, assignment taskqueue.TaskAssignment) (taskTransitionCurrentResult, error) {
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
	if rule.requiresLease && !assignment.HasLeaseIdentity() {
		return taskTransitionCurrentResult{}, ErrTaskStateConflict
	}
	if task.Status == tasks.StatusRunning && rule.requiresLease && !taskMatchesLeaseAssignment(task, assignment) {
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

func taskMatchesLeaseAssignment(task *tasks.Task, assignment taskqueue.TaskAssignment) bool {
	if task == nil {
		return false
	}
	return tasks.IDValue(task.AccountID) == assignment.AccountID &&
		tasks.IDValue(task.AssignedProviderEndpointID) == assignment.ProviderEndpointID &&
		tasks.IDValue(task.AssignedCredentialID) == assignment.CredentialID &&
		task.LeaseToken == assignment.LeaseToken
}
