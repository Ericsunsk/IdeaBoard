package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
)

type nonterminalTransitionPolicy struct {
	attemptStatus string
	leaseReason   string
}

type nonterminalTransitionCommand struct {
	taskID        string
	kind          TaskConvergenceKind
	occurredAt    time.Time
	errorMessage  string
	result        string
	attemptFinish *tasks.TaskAttemptFinish
	lease         terminalLeaseRelease
	policy        nonterminalTransitionPolicy
}

func resolveNonterminalTransitionPolicy(kind TaskConvergenceKind) (nonterminalTransitionPolicy, error) {
	switch kind {
	case TaskConvergenceRequeued:
		return nonterminalTransitionPolicy{attemptStatus: tasks.TaskAttemptStatusRequeued, leaseReason: tasks.TaskAttemptStatusRequeued}, nil
	case TaskConvergenceReset:
		return nonterminalTransitionPolicy{attemptStatus: tasks.TaskAttemptStatusRequeued, leaseReason: "reset"}, nil
	default:
		return nonterminalTransitionPolicy{}, fmt.Errorf("unsupported nonterminal task convergence %q", kind)
	}
}

func (r *Repository) runNonterminalTransition(ctx context.Context, command nonterminalTransitionCommand) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("task convergence repository is unavailable")
	}
	command.taskID = strings.TrimSpace(command.taskID)
	if command.taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	if command.occurredAt.IsZero() {
		command.occurredAt = time.Now()
	}

	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		normalizedAttempt, err := normalizeNonterminalAttemptFinish(command)
		if err != nil {
			return err
		}
		updated, err := writeNonterminalTaskFact(ctx, q, command)
		if err != nil || updated == 0 {
			return err
		}
		if err := finishNonterminalTaskAttemptTx(ctx, q, command, normalizedAttempt); err != nil {
			return err
		}
		if err := releaseTerminalTaskLeaseTx(ctx, q, terminalTransitionCommand{
			taskID:     command.taskID,
			finishedAt: command.occurredAt,
			lease:      command.lease,
		}, terminalTransitionPolicy{leaseReason: command.policy.leaseReason}); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func finishNonterminalTaskAttemptTx(ctx context.Context, q *dbsqlc.Queries, command nonterminalTransitionCommand, finish *tasks.TaskAttemptFinish) error {
	if finish != nil && finish.ID > 0 {
		return finishTaskAttemptTx(ctx, q, finish)
	}
	_, err := q.FinishRunningTaskAttemptsByTaskIDs(ctx, dbsqlc.FinishRunningTaskAttemptsByTaskIDsParams{
		Status:          command.policy.attemptStatus,
		FailureStage:    tasks.ExecutionStageAssigned,
		FailureCategory: tasks.FailureCategoryUnknown,
		ErrorMsg:        strings.TrimSpace(command.errorMessage),
		FinishedAt:      sql.NullTime{Time: command.occurredAt, Valid: true},
		TaskIds:         []string{command.taskID},
	})
	return err
}

func normalizeNonterminalAttemptFinish(command nonterminalTransitionCommand) (*tasks.TaskAttemptFinish, error) {
	if command.attemptFinish == nil || command.attemptFinish.ID == 0 {
		return command.attemptFinish, nil
	}
	finish := *command.attemptFinish
	if status := strings.TrimSpace(finish.Status); status != "" && status != command.policy.attemptStatus {
		return nil, fmt.Errorf("task attempt nonterminal status=%q conflicts with policy status=%q", status, command.policy.attemptStatus)
	}
	finish.Status = command.policy.attemptStatus
	return taskAttemptFinishWithLeaseIdentity(
		&finish,
		command.taskID,
		command.lease.accountID,
		command.lease.providerEndpointID,
		command.lease.credentialID,
		command.lease.leaseToken,
	)
}

func writeNonterminalTaskFact(ctx context.Context, q *dbsqlc.Queries, command nonterminalTransitionCommand) (int64, error) {
	switch command.kind {
	case TaskConvergenceRequeued:
		return q.RequeueTaskRunningWithLease(ctx, dbsqlc.RequeueTaskRunningWithLeaseParams{
			ErrorMsg:                   command.errorMessage,
			Result:                     command.result,
			TaskID:                     command.taskID,
			Statuses:                   []string{tasks.StatusRunning},
			AccountID:                  dbutil.NullInt64(&command.lease.accountID),
			AssignedProviderEndpointID: dbutil.NullInt64(&command.lease.providerEndpointID),
			AssignedCredentialID:       dbutil.NullInt64(&command.lease.credentialID),
			LeaseToken:                 command.lease.leaseToken,
		})
	case TaskConvergenceReset:
		return q.ResetTaskRunningToPendingWithLease(ctx, dbsqlc.ResetTaskRunningToPendingWithLeaseParams{
			ErrorMsg:                   command.errorMessage,
			TaskID:                     command.taskID,
			Statuses:                   []string{tasks.StatusRunning},
			AccountID:                  dbutil.NullInt64(&command.lease.accountID),
			AssignedProviderEndpointID: dbutil.NullInt64(&command.lease.providerEndpointID),
			AssignedCredentialID:       dbutil.NullInt64(&command.lease.credentialID),
			LeaseToken:                 command.lease.leaseToken,
		})
	default:
		return 0, fmt.Errorf("unsupported nonterminal task convergence %q", command.kind)
	}
}
