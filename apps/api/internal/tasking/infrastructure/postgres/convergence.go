package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	"github.com/jackc/pgx/v5/pgtype"
)

type nonterminalTransitionPolicy struct {
	attemptStatus string
}

type nonterminalTransitionCommand struct {
	taskID         string
	kind           runtimecontracts.TaskConvergenceKind
	occurredAt     time.Time
	errorMessage   string
	result         string
	attemptFinish  *tasks.TaskAttemptFinish
	executionToken string
	policy         nonterminalTransitionPolicy
}

func resolveNonterminalTransitionPolicy(kind runtimecontracts.TaskConvergenceKind) (nonterminalTransitionPolicy, error) {
	switch kind {
	case runtimecontracts.TaskConvergenceRequeued:
		return nonterminalTransitionPolicy{attemptStatus: tasks.TaskAttemptStatusRequeued}, nil
	case runtimecontracts.TaskConvergenceReset:
		return nonterminalTransitionPolicy{attemptStatus: tasks.TaskAttemptStatusRequeued}, nil
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
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func finishNonterminalTaskAttemptTx(ctx context.Context, q *dbsqlc.Queries, command nonterminalTransitionCommand, finish *tasks.TaskAttemptFinish) error {
	status := command.policy.attemptStatus
	stage := tasks.ExecutionStageAssigned
	category := tasks.FailureCategoryUnknown
	if finish != nil {
		if s := strings.TrimSpace(finish.Status); s != "" {
			status = s
		}
		if s := strings.TrimSpace(finish.FailureStage); s != "" {
			stage = s
		}
		if s := strings.TrimSpace(finish.FailureCategory); s != "" {
			category = s
		}
	}
	if token := strings.TrimSpace(command.executionToken); token != "" {
		affected, err := q.FinalizeExecutionAttempt(ctx, dbsqlc.FinalizeExecutionAttemptParams{
			Status:          status,
			FailureStage:    stage,
			FailureCategory: category,
			ErrorMsg:        strings.TrimSpace(command.errorMessage),
			CompletedAt:     pgtype.Timestamptz{Time: command.occurredAt, Valid: true},
			ExecutionToken:  token,
			TaskID:          command.taskID,
		})
		if err != nil {
			return err
		}
		// Task fact fencing above is authoritative: the token already proved
		// ownership of the running task. Zero affected rows only means no
		// running attempt row carries this token (e.g. Claim succeeded but
		// Start failed), so the reset/requeue still stands without an
		// attempt row to close.
		_ = affected
		return nil
	}
	_, err := q.FinishRunningExecutionAttemptsByTaskIDs(ctx, dbsqlc.FinishRunningExecutionAttemptsByTaskIDsParams{
		Status:          status,
		FailureStage:    stage,
		FailureCategory: category,
		ErrorMsg:        strings.TrimSpace(command.errorMessage),
		FinishedAt:      pgtype.Timestamptz{Time: command.occurredAt, Valid: true},
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
	if strings.TrimSpace(finish.TaskID) == "" {
		finish.TaskID = command.taskID
	}
	return &finish, nil
}

func writeNonterminalTaskFact(ctx context.Context, q *dbsqlc.Queries, command nonterminalTransitionCommand) (int64, error) {
	token := strings.TrimSpace(command.executionToken)
	if token == "" {
		return 0, fmt.Errorf("%w: pending convergence requires an execution token", runtimecontracts.ErrTaskLeaseStateConflict)
	}
	switch command.kind {
	case runtimecontracts.TaskConvergenceRequeued:
		return q.RequeueTaskRunningWithExecutionToken(ctx, dbsqlc.RequeueTaskRunningWithExecutionTokenParams{
			ErrorMsg:       command.errorMessage,
			Result:         command.result,
			TaskID:         command.taskID,
			Statuses:       []string{tasks.StatusRunning},
			ExecutionToken: token,
		})
	case runtimecontracts.TaskConvergenceReset:
		return q.ResetTaskExecutionToken(ctx, dbsqlc.ResetTaskExecutionTokenParams{
			ErrorMsg:       command.errorMessage,
			TaskID:         command.taskID,
			Statuses:       []string{tasks.StatusRunning},
			ExecutionToken: token,
		})
	default:
		return 0, fmt.Errorf("unsupported nonterminal task convergence %q", command.kind)
	}
}
