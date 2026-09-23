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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type terminalTransitionKind string

const (
	terminalTransitionCompleted terminalTransitionKind = "completed"
	terminalTransitionFailed    terminalTransitionKind = "failed"
	terminalTransitionCancelled terminalTransitionKind = "cancelled"
)

type terminalTransitionCommand struct {
	taskID          string
	kind            terminalTransitionKind
	finishedAt      time.Time
	errorMessage    string
	failureStage    string
	failureCategory string
	attemptFinish   *tasks.TaskAttemptFinish
	// executionToken fences terminal writes. Empty means a system-initiated
	// transition that needs no fence; the attempt sweep still converges
	// straggler rows by task.
	executionToken string
	// Gateway receipt facts recorded on the closed attempt. The orchestrator
	// supplies them; the terminal transaction is the single writer so
	// concurrent workers cannot split attempt state.
	resolvedModel  string
	gatewayStep    string
	receiptLogID   string
	receiptEventID string
	cacheHit       bool
	outputs        []tasks.TaskOutput
	writeTaskFact  func(context.Context, *dbsqlc.Queries) (int64, error)
}

type terminalTransitionPolicy struct {
	taskStatus      string
	attemptStatus   string
	executionStage  string
	failureStage    string
	failureCategory string
	replacesOutputs bool
}

// ConvergeTask is the single Task runtime write seam. It commits the Task
// fact, attempt and exact lease release as one database fact; terminal kinds
// additionally commit metrics, daily stats and Credit settlement.
func (r *Repository) ConvergeTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (rowsAffected int64, err error) {
	switch intent.Kind {
	case runtimecontracts.TaskConvergenceCompleted:
		return r.convergeCompletedTask(ctx, intent)
	case runtimecontracts.TaskConvergenceFailed, runtimecontracts.TaskConvergenceFinalFailed, runtimecontracts.TaskConvergenceRecoveredFailed:
		return r.convergeFailedTask(ctx, intent)
	case runtimecontracts.TaskConvergenceRequeued, runtimecontracts.TaskConvergenceReset:
		return r.convergePendingTask(ctx, intent)
	default:
		return 0, fmt.Errorf("unsupported task convergence kind %q", intent.Kind)
	}
}

func (r *Repository) convergeCompletedTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error) {
	finishedAt := terminalTransitionTime(intent.OccurredAt)
	token := strings.TrimSpace(intent.ExecutionToken)
	if token == "" {
		return 0, fmt.Errorf("%w: completed convergence requires an execution token", runtimecontracts.ErrTaskLeaseStateConflict)
	}
	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:         intent.TaskID,
		kind:           terminalTransitionCompleted,
		finishedAt:     finishedAt,
		attemptFinish:  intent.AttemptFinish,
		outputs:        intent.Outputs,
		executionToken: token,
		resolvedModel:  strings.TrimSpace(intent.ResolvedModel),
		gatewayStep:    strings.TrimSpace(intent.GatewayStep),
		receiptLogID:   strings.TrimSpace(intent.ReceiptLogID),
		receiptEventID: strings.TrimSpace(intent.ReceiptEventID),
		cacheHit:       intent.CacheHit,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			return q.CompleteTaskWithExecutionToken(ctx, dbsqlc.CompleteTaskWithExecutionTokenParams{
				Result:         intent.Result,
				CompletedAt:    pgtype.Timestamptz{Time: finishedAt, Valid: true},
				TaskID:         intent.TaskID,
				Statuses:       []string{tasks.StatusRunning},
				ExecutionToken: token,
			})
		},
	})
}

func (r *Repository) convergeFailedTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error) {
	finishedAt := terminalTransitionTime(intent.OccurredAt)
	token := strings.TrimSpace(intent.ExecutionToken)
	if token == "" {
		return r.runTerminalTransition(ctx, terminalTransitionCommand{
			taskID:        intent.TaskID,
			kind:          terminalTransitionFailed,
			finishedAt:    finishedAt,
			errorMessage:  intent.ErrorMessage,
			attemptFinish: intent.AttemptFinish,
			writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
				if intent.Kind == runtimecontracts.TaskConvergenceRecoveredFailed {
					return q.MarkUnleasedTaskFailed(ctx, dbsqlc.MarkUnleasedTaskFailedParams{
						ErrorMsg:    intent.ErrorMessage,
						Result:      intent.Result,
						CompletedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
						TaskID:      intent.TaskID,
						Statuses:    []string{tasks.StatusPending, tasks.StatusRunning},
					})
				}
				// Pending-only fail rides the same guarded query: pending tasks
				// never hold a running attempt, so NOT EXISTS is vacuous here
				// and a stray running attempt blocks a wrong terminal write.
				return q.MarkUnleasedTaskFailed(ctx, dbsqlc.MarkUnleasedTaskFailedParams{
					ErrorMsg:    intent.ErrorMessage,
					Result:      intent.Result,
					CompletedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
					TaskID:      intent.TaskID,
					Statuses:    []string{tasks.StatusPending},
				})
			},
		})
	}

	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:         intent.TaskID,
		kind:           terminalTransitionFailed,
		finishedAt:     finishedAt,
		errorMessage:   intent.ErrorMessage,
		attemptFinish:  intent.AttemptFinish,
		executionToken: token,
		resolvedModel:  strings.TrimSpace(intent.ResolvedModel),
		gatewayStep:    strings.TrimSpace(intent.GatewayStep),
		receiptLogID:   strings.TrimSpace(intent.ReceiptLogID),
		receiptEventID: strings.TrimSpace(intent.ReceiptEventID),
		cacheHit:       intent.CacheHit,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			if intent.Kind == runtimecontracts.TaskConvergenceFinalFailed {
				return q.FinalFailTaskRunningWithExecutionToken(ctx, dbsqlc.FinalFailTaskRunningWithExecutionTokenParams{
					ErrorMsg:       intent.ErrorMessage,
					Result:         intent.Result,
					CompletedAt:    pgtype.Timestamptz{Time: finishedAt, Valid: true},
					TaskID:         intent.TaskID,
					Statuses:       []string{tasks.StatusRunning},
					ExecutionToken: token,
				})
			}
			return q.MarkTaskFailedWithExecutionToken(ctx, dbsqlc.MarkTaskFailedWithExecutionTokenParams{
				ErrorMsg:       intent.ErrorMessage,
				Result:         intent.Result,
				CompletedAt:    pgtype.Timestamptz{Time: finishedAt, Valid: true},
				TaskID:         intent.TaskID,
				Statuses:       []string{tasks.StatusRunning},
				ExecutionToken: token,
			})
		},
	})
}

func (r *Repository) convergePendingTask(ctx context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error) {
	token := strings.TrimSpace(intent.ExecutionToken)
	if token == "" {
		return 0, fmt.Errorf("%w: pending convergence requires an execution token", runtimecontracts.ErrTaskLeaseStateConflict)
	}
	policy, err := resolveNonterminalTransitionPolicy(intent.Kind)
	if err != nil {
		return 0, err
	}
	return r.runNonterminalTransition(ctx, nonterminalTransitionCommand{
		taskID:         intent.TaskID,
		kind:           intent.Kind,
		occurredAt:     terminalTransitionTime(intent.OccurredAt),
		errorMessage:   intent.ErrorMessage,
		result:         intent.Result,
		attemptFinish:  intent.AttemptFinish,
		executionToken: token,
		policy:         policy,
	})
}

// runTerminalTransition owns the atomic terminal recipe. Repository methods
// only adapt their caller-specific task fact command into this invariant.
func (r *Repository) runTerminalTransition(ctx context.Context, command terminalTransitionCommand) (int64, error) {
	command, policy, err := prepareTerminalTransition(r, command)
	if err != nil {
		return 0, err
	}
	return r.runTerminalTransitionTx(ctx, command, policy)
}

func prepareTerminalTransition(r *Repository, command terminalTransitionCommand) (terminalTransitionCommand, terminalTransitionPolicy, error) {
	if r == nil || r.db == nil {
		return terminalTransitionCommand{}, terminalTransitionPolicy{}, fmt.Errorf("task terminal repository is unavailable")
	}
	command.taskID = strings.TrimSpace(command.taskID)
	if command.taskID == "" {
		return terminalTransitionCommand{}, terminalTransitionPolicy{}, fmt.Errorf("task_id is required")
	}
	if command.writeTaskFact == nil {
		return terminalTransitionCommand{}, terminalTransitionPolicy{}, fmt.Errorf("task terminal fact writer is required")
	}
	if command.finishedAt.IsZero() {
		command.finishedAt = time.Now()
	}
	policy, err := resolveTerminalTransitionPolicy(command.kind)
	if err != nil {
		return terminalTransitionCommand{}, terminalTransitionPolicy{}, err
	}
	return command, policy, nil
}

func (r *Repository) runTerminalTransitionTx(ctx context.Context, command terminalTransitionCommand, policy terminalTransitionPolicy) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		updated, err := applyTerminalTransitionTx(ctx, q, tx, command, policy)
		if err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func applyTerminalTransitionTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	tx pgx.Tx,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) (int64, error) {
	normalizedAttemptFinish, err := normalizeTerminalAttemptFinishTx(command, policy)
	if err != nil {
		return 0, err
	}
	command.attemptFinish = normalizedAttemptFinish

	updated, err := command.writeTaskFact(ctx, q)
	if err != nil || updated == 0 {
		return updated, err
	}

	taskRow, err := q.GetTaskByTaskID(ctx, command.taskID)
	if err != nil {
		return 0, err
	}
	task := taskFromSQLC(taskRow)
	if err := validateTerminalTaskStatus(command, policy, task.Status); err != nil {
		return 0, err
	}
	if err := finalizeTerminalTaskTx(ctx, q, tx, &task, command, policy); err != nil {
		return 0, err
	}
	return updated, nil
}

func validateTerminalTaskStatus(command terminalTransitionCommand, policy terminalTransitionPolicy, status string) error {
	isCompatibleCancellation := command.kind == terminalTransitionCancelled && tasks.IsTerminalStatus(status) && (status == tasks.StatusCancelled || status == tasks.StatusCancelledWithResults)
	if status != policy.taskStatus && !isCompatibleCancellation {
		return fmt.Errorf("task terminal fact status=%q, want %q", status, policy.taskStatus)
	}
	return nil
}

func finalizeTerminalTaskTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	tx pgx.Tx,
	task *tasks.Task,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) error {
	if policy.replacesOutputs {
		if err := replaceTaskOutputsTx(ctx, q, task, command.outputs); err != nil {
			return err
		}
	}
	if err := finishTerminalTaskAttemptTx(ctx, q, command, policy); err != nil {
		return err
	}
	// No lease release step: finalizing the execution attempt IS the release.
	// A token-fenced attempt closes exactly; the by-task sweep converges
	// stragglers left by crashed workers.
	if _, err := q.ReleaseTaskInputMediaRefs(ctx, command.taskID); err != nil {
		return err
	}
	metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, []string{command.taskID})
	if err != nil {
		return err
	}
	return applyTerminalOutcomeTx(ctx, q, tx, task, metricsByTaskID[command.taskID], command, policy)
}

func applyTerminalOutcomeTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	tx pgx.Tx,
	task *tasks.Task,
	metrics *tasks.TaskExecutionMetrics,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) error {
	// The task snapshot above predates attempt finalization, so receipt
	// facts live only on the command at this point. Overlay them before
	// the stat write or daily stats permanently record "unknown model".
	if model := strings.TrimSpace(command.resolvedModel); model != "" {
		task.ResolvedModel = model
	}
	if err := upsertDailyStatByTaskTx(ctx, q, task, command.attemptFinish); err != nil {
		return err
	}
	if err := patchTerminalExecutionMetricsForTaskTx(
		ctx,
		q,
		command.taskID,
		task.CreatedAt,
		metrics,
		policy.executionStage,
		terminalCommandFailureStage(command, policy),
		terminalCommandFailureCategory(command, policy),
		command.finishedAt,
	); err != nil {
		return err
	}
	return settleTerminal(ctx, tx, command.taskID)
}

func resolveTerminalTransitionPolicy(kind terminalTransitionKind) (terminalTransitionPolicy, error) {
	switch kind {
	case terminalTransitionCompleted:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusCompleted,
			attemptStatus:   tasks.TaskAttemptStatusCompleted,
			executionStage:  tasks.ExecutionStageCompleted,
			replacesOutputs: true,
		}, nil
	case terminalTransitionFailed:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusFailed,
			attemptStatus:   tasks.TaskAttemptStatusFailed,
			executionStage:  tasks.ExecutionStageFailed,
			failureStage:    tasks.ExecutionStageAssigned,
			failureCategory: tasks.FailureCategoryUnknown,
		}, nil
	case terminalTransitionCancelled:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusCancelled,
			attemptStatus:   tasks.TaskAttemptStatusCancelled,
			executionStage:  tasks.ExecutionStageCancelled,
			failureStage:    tasks.ExecutionStageQueued,
			failureCategory: tasks.FailureCategoryCancelled,
		}, nil
	default:
		return terminalTransitionPolicy{}, fmt.Errorf("unsupported task terminal transition %q", kind)
	}
}

func replaceTaskOutputsTx(ctx context.Context, q *dbsqlc.Queries, task *tasks.Task, outputs []tasks.TaskOutput) error {
	if err := q.DeleteTaskOutputMediaRefsByTaskID(ctx, task.TaskID); err != nil {
		return err
	}
	if err := q.DeleteOutputsByTaskID(ctx, task.TaskID); err != nil {
		return err
	}
	return insertTaskOutputs(ctx, q, task.TaskID, task.UserID, outputs)
}

func finishTerminalTaskAttemptTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) error {
	token := strings.TrimSpace(command.executionToken)
	if token != "" {
		_, err := q.FinalizeExecutionAttempt(ctx, dbsqlc.FinalizeExecutionAttemptParams{
			Status:             policy.attemptStatus,
			ResolvedModel:      strings.TrimSpace(command.resolvedModel),
			GatewayStep:        strings.TrimSpace(command.gatewayStep),
			ReceiptLogID:       strings.TrimSpace(command.receiptLogID),
			ReceiptEventID:     strings.TrimSpace(command.receiptEventID),
			CacheHit:           command.cacheHit,
			FailureStage:       terminalCommandFailureStage(command, policy),
			FailureCategory:    terminalCommandFailureCategory(command, policy),
			ErrorMsg:           terminalCommandErrorMessage(command, policy),
			ProviderFinishedAt: pgtype.Timestamptz{},
			CompletedAt:        pgtype.Timestamptz{Time: command.finishedAt, Valid: true},
			ExecutionToken:     token,
			TaskID:             command.taskID,
		})
		return err
	}
	_, err := q.FinishRunningExecutionAttemptsByTaskIDs(ctx, dbsqlc.FinishRunningExecutionAttemptsByTaskIDsParams{
		Status:          policy.attemptStatus,
		FailureStage:    terminalCommandFailureStage(command, policy),
		FailureCategory: terminalCommandFailureCategory(command, policy),
		ErrorMsg:        terminalCommandErrorMessage(command, policy),
		FinishedAt:      pgtype.Timestamptz{Time: command.finishedAt, Valid: true},
		TaskIds:         []string{command.taskID},
	})
	return err
}

func terminalCommandErrorMessage(command terminalTransitionCommand, policy terminalTransitionPolicy) string {
	errorMessage := strings.TrimSpace(command.errorMessage)
	if errorMessage == "" && policy.attemptStatus != tasks.TaskAttemptStatusCompleted {
		errorMessage = "task " + string(command.kind)
	}
	return errorMessage
}

func normalizeTerminalAttemptFinishTx(
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) (*tasks.TaskAttemptFinish, error) {
	if command.attemptFinish == nil || command.attemptFinish.ID == 0 {
		return command.attemptFinish, nil
	}
	finish := *command.attemptFinish
	if callerStatus := strings.TrimSpace(finish.Status); callerStatus != "" && callerStatus != policy.attemptStatus {
		return nil, fmt.Errorf(
			"task attempt terminal status=%q conflicts with policy status=%q",
			callerStatus,
			policy.attemptStatus,
		)
	}
	finish.Status = policy.attemptStatus
	if strings.TrimSpace(finish.TaskID) == "" {
		finish.TaskID = command.taskID
	}
	return &finish, nil
}

func terminalCommandFailureStage(command terminalTransitionCommand, policy terminalTransitionPolicy) string {
	if value := strings.TrimSpace(command.failureStage); value != "" {
		return value
	}
	if command.attemptFinish != nil {
		if value := strings.TrimSpace(command.attemptFinish.FailureStage); value != "" {
			return value
		}
	}
	return policy.failureStage
}

func terminalCommandFailureCategory(command terminalTransitionCommand, policy terminalTransitionPolicy) string {
	if value := strings.TrimSpace(command.failureCategory); value != "" {
		return value
	}
	if command.attemptFinish != nil {
		if value := strings.TrimSpace(command.attemptFinish.FailureCategory); value != "" {
			return value
		}
	}
	return policy.failureCategory
}

func terminalTransitionTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
