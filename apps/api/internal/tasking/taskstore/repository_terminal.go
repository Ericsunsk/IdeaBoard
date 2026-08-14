package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
)

type terminalTransitionKind string

const (
	terminalTransitionCompleted terminalTransitionKind = "completed"
	terminalTransitionFailed    terminalTransitionKind = "failed"
	terminalTransitionCancelled terminalTransitionKind = "cancelled"
)

type terminalLeaseReleaseMode uint8

const (
	terminalLeaseReleaseNone terminalLeaseReleaseMode = iota
	terminalLeaseReleaseExact
	terminalLeaseReleaseActive
)

type terminalLeaseRelease struct {
	mode               terminalLeaseReleaseMode
	accountID          int64
	providerEndpointID int64
	credentialID       int64
	leaseToken         string
}

type terminalTransitionCommand struct {
	taskID          string
	kind            terminalTransitionKind
	finishedAt      time.Time
	errorMessage    string
	failureStage    string
	failureCategory string
	attemptFinish   *tasks.TaskAttemptFinish
	lease           terminalLeaseRelease
	outputs         []tasks.TaskOutput
	writeTaskFact   func(context.Context, *dbsqlc.Queries) (int64, error)
}

type terminalTransitionPolicy struct {
	taskStatus      string
	attemptStatus   string
	executionStage  string
	failureStage    string
	failureCategory string
	leaseReason     string
	replacesOutputs bool
}

type TaskConvergenceKind string

const (
	TaskConvergenceCompleted       TaskConvergenceKind = "completed"
	TaskConvergenceFailed          TaskConvergenceKind = "failed"
	TaskConvergenceFinalFailed     TaskConvergenceKind = "final_failed"
	TaskConvergenceRecoveredFailed TaskConvergenceKind = "recovered_failed"
	TaskConvergenceRequeued        TaskConvergenceKind = "requeued"
	TaskConvergenceReset           TaskConvergenceKind = "reset"
)

type TaskLeaseIdentity struct {
	AccountID          int64
	ProviderEndpointID int64
	CredentialID       int64
	LeaseToken         string
}

type TaskConvergenceIntent struct {
	Kind          TaskConvergenceKind
	TaskID        string
	Lease         *TaskLeaseIdentity
	Result        string
	ErrorMessage  string
	OccurredAt    time.Time
	Outputs       []tasks.TaskOutput
	AttemptFinish *tasks.TaskAttemptFinish
}

// ConvergeTask is the single Task runtime write seam. It commits the Task
// fact, attempt and exact lease release as one database fact; terminal kinds
// additionally commit metrics, daily stats and Credit settlement.
func (r *Repository) ConvergeTask(ctx context.Context, intent TaskConvergenceIntent) (int64, error) {
	switch intent.Kind {
	case TaskConvergenceCompleted:
		return r.convergeCompletedTask(ctx, intent)
	case TaskConvergenceFailed, TaskConvergenceFinalFailed, TaskConvergenceRecoveredFailed:
		return r.convergeFailedTask(ctx, intent)
	case TaskConvergenceRequeued, TaskConvergenceReset:
		return r.convergePendingTask(ctx, intent)
	default:
		return 0, fmt.Errorf("unsupported task convergence kind %q", intent.Kind)
	}
}

func (r *Repository) convergeCompletedTask(ctx context.Context, intent TaskConvergenceIntent) (int64, error) {
	finishedAt := terminalTransitionTime(intent.OccurredAt)
	lease, err := terminalLeaseReleaseForIdentity(intent.Lease)
	if err != nil {
		return 0, err
	}
	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:        intent.TaskID,
		kind:          terminalTransitionCompleted,
		finishedAt:    finishedAt,
		attemptFinish: intent.AttemptFinish,
		outputs:       intent.Outputs,
		lease:         lease,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			return q.CompleteTaskWithLease(ctx, completeTaskWithLeaseParams(intent, finishedAt))
		},
	})
}

func (r *Repository) convergeFailedTask(ctx context.Context, intent TaskConvergenceIntent) (int64, error) {
	finishedAt := terminalTransitionTime(intent.OccurredAt)
	if intent.Lease == nil {
		return r.runTerminalTransition(ctx, terminalTransitionCommand{
			taskID:        intent.TaskID,
			kind:          terminalTransitionFailed,
			finishedAt:    finishedAt,
			errorMessage:  intent.ErrorMessage,
			attemptFinish: intent.AttemptFinish,
			writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
				if intent.Kind == TaskConvergenceRecoveredFailed {
					return q.MarkUnleasedTaskFailed(ctx, dbsqlc.MarkUnleasedTaskFailedParams{
						ErrorMsg:    intent.ErrorMessage,
						Result:      intent.Result,
						CompletedAt: sql.NullTime{Time: finishedAt, Valid: true},
						TaskID:      intent.TaskID,
						Statuses:    []string{tasks.StatusPending, tasks.StatusRunning},
					})
				}
				return q.MarkTaskFailed(ctx, dbsqlc.MarkTaskFailedParams{
					ErrorMsg:    intent.ErrorMessage,
					Result:      intent.Result,
					CompletedAt: sql.NullTime{Time: finishedAt, Valid: true},
					TaskID:      intent.TaskID,
					Statuses:    []string{tasks.StatusPending},
				})
			},
		})
	}

	lease, err := terminalLeaseReleaseForIdentity(intent.Lease)
	if err != nil {
		return 0, err
	}
	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:        intent.TaskID,
		kind:          terminalTransitionFailed,
		finishedAt:    finishedAt,
		errorMessage:  intent.ErrorMessage,
		attemptFinish: intent.AttemptFinish,
		lease:         lease,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			if intent.Kind == TaskConvergenceFinalFailed {
				return q.FinalFailTaskRunningWithLease(ctx, finalFailTaskWithLeaseParams(intent, finishedAt))
			}
			return q.MarkTaskFailedWithLease(ctx, failedTaskWithLeaseParams(intent, finishedAt))
		},
	})
}

func (r *Repository) convergePendingTask(ctx context.Context, intent TaskConvergenceIntent) (int64, error) {
	lease, err := terminalLeaseReleaseForIdentity(intent.Lease)
	if err != nil {
		return 0, err
	}
	policy, err := resolveNonterminalTransitionPolicy(intent.Kind)
	if err != nil {
		return 0, err
	}
	return r.runNonterminalTransition(ctx, nonterminalTransitionCommand{
		taskID:        intent.TaskID,
		kind:          intent.Kind,
		occurredAt:    terminalTransitionTime(intent.OccurredAt),
		errorMessage:  intent.ErrorMessage,
		result:        intent.Result,
		attemptFinish: intent.AttemptFinish,
		lease:         lease,
		policy:        policy,
	})
}

func terminalLeaseReleaseForIdentity(identity *TaskLeaseIdentity) (terminalLeaseRelease, error) {
	if identity == nil {
		return terminalLeaseRelease{}, fmt.Errorf("%w: task convergence requires lease identity", ErrTaskLeaseStateConflict)
	}
	lease := terminalLeaseRelease{
		mode:               terminalLeaseReleaseExact,
		accountID:          identity.AccountID,
		providerEndpointID: identity.ProviderEndpointID,
		credentialID:       identity.CredentialID,
		leaseToken:         identity.LeaseToken,
	}
	if lease.accountID <= 0 || lease.providerEndpointID <= 0 || lease.credentialID <= 0 || strings.TrimSpace(lease.leaseToken) == "" {
		return terminalLeaseRelease{}, fmt.Errorf("%w: task convergence requires lease identity", ErrTaskLeaseStateConflict)
	}
	return lease, nil
}

func completeTaskWithLeaseParams(intent TaskConvergenceIntent, completedAt time.Time) dbsqlc.CompleteTaskWithLeaseParams {
	return dbsqlc.CompleteTaskWithLeaseParams{
		Result:                     intent.Result,
		CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
		TaskID:                     intent.TaskID,
		Statuses:                   []string{tasks.StatusRunning},
		AccountID:                  sql.NullInt64{Int64: intent.Lease.AccountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: intent.Lease.ProviderEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: intent.Lease.CredentialID, Valid: true},
		LeaseToken:                 intent.Lease.LeaseToken,
	}
}

func failedTaskWithLeaseParams(intent TaskConvergenceIntent, completedAt time.Time) dbsqlc.MarkTaskFailedWithLeaseParams {
	return dbsqlc.MarkTaskFailedWithLeaseParams{
		ErrorMsg:                   intent.ErrorMessage,
		Result:                     intent.Result,
		CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
		TaskID:                     intent.TaskID,
		Statuses:                   []string{tasks.StatusRunning},
		AccountID:                  sql.NullInt64{Int64: intent.Lease.AccountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: intent.Lease.ProviderEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: intent.Lease.CredentialID, Valid: true},
		LeaseToken:                 intent.Lease.LeaseToken,
	}
}

func finalFailTaskWithLeaseParams(intent TaskConvergenceIntent, completedAt time.Time) dbsqlc.FinalFailTaskRunningWithLeaseParams {
	return dbsqlc.FinalFailTaskRunningWithLeaseParams{
		ErrorMsg:                   intent.ErrorMessage,
		Result:                     intent.Result,
		CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
		TaskID:                     intent.TaskID,
		Statuses:                   []string{tasks.StatusRunning},
		AccountID:                  sql.NullInt64{Int64: intent.Lease.AccountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: intent.Lease.ProviderEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: intent.Lease.CredentialID, Valid: true},
		LeaseToken:                 intent.Lease.LeaseToken,
	}
}

// runTerminalTransition owns the atomic terminal recipe. Repository methods
// only adapt their caller-specific task fact command into this invariant.
func (r *Repository) runTerminalTransition(ctx context.Context, command terminalTransitionCommand) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("task terminal repository is unavailable")
	}
	command.taskID = strings.TrimSpace(command.taskID)
	if command.taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	if command.writeTaskFact == nil {
		return 0, fmt.Errorf("task terminal fact writer is required")
	}
	if command.finishedAt.IsZero() {
		command.finishedAt = time.Now()
	}
	policy, err := resolveTerminalTransitionPolicy(command.kind)
	if err != nil {
		return 0, err
	}

	var rowsAffected int64
	err = dbutil.WithTxDBTX(ctx, r.db, func(q *dbsqlc.Queries, tx *sql.Tx) error {
		normalizedAttemptFinish, err := normalizeTerminalAttemptFinishTx(ctx, q, command, policy)
		if err != nil {
			return err
		}
		command.attemptFinish = normalizedAttemptFinish

		updated, err := command.writeTaskFact(ctx, q)
		if err != nil || updated == 0 {
			return err
		}

		taskRow, err := q.GetTaskByTaskID(ctx, command.taskID)
		if err != nil {
			return err
		}
		task := taskFromSQLC(taskRow)
		isCompatibleCancellation := command.kind == terminalTransitionCancelled && tasks.IsTerminalStatus(task.Status) && (task.Status == tasks.StatusCancelled || task.Status == tasks.StatusCancelledWithResults)
		if task.Status != policy.taskStatus && !isCompatibleCancellation {
			return fmt.Errorf("task terminal fact status=%q, want %q", task.Status, policy.taskStatus)
		}

		if policy.replacesOutputs {
			if err := replaceTaskOutputsTx(ctx, q, &task, command.outputs); err != nil {
				return err
			}
		}
		if err := finishTerminalTaskAttemptTx(ctx, q, command, policy); err != nil {
			return err
		}
		if err := releaseTerminalTaskLeaseTx(ctx, q, command, policy); err != nil {
			return err
		}
		metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, []string{command.taskID})
		if err != nil {
			return err
		}
		if err := applyTerminalOutcomeTx(ctx, q, tx, &task, metricsByTaskID[command.taskID], command, policy); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func applyTerminalOutcomeTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	tx *sql.Tx,
	task *tasks.Task,
	metrics *tasks.TaskExecutionMetrics,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) error {
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
	return taskbilling.SettleTerminal(ctx, tx, command.taskID)
}

func resolveTerminalTransitionPolicy(kind terminalTransitionKind) (terminalTransitionPolicy, error) {
	switch kind {
	case terminalTransitionCompleted:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusCompleted,
			attemptStatus:   tasks.TaskAttemptStatusCompleted,
			executionStage:  tasks.ExecutionStageCompleted,
			leaseReason:     tasks.TaskAttemptStatusCompleted,
			replacesOutputs: true,
		}, nil
	case terminalTransitionFailed:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusFailed,
			attemptStatus:   tasks.TaskAttemptStatusFailed,
			executionStage:  tasks.ExecutionStageFailed,
			failureStage:    tasks.ExecutionStageAssigned,
			failureCategory: tasks.FailureCategoryUnknown,
			leaseReason:     tasks.TaskAttemptStatusFailed,
		}, nil
	case terminalTransitionCancelled:
		return terminalTransitionPolicy{
			taskStatus:      tasks.StatusCancelled,
			attemptStatus:   tasks.TaskAttemptStatusCancelled,
			executionStage:  tasks.ExecutionStageCancelled,
			failureStage:    tasks.ExecutionStageQueued,
			failureCategory: tasks.FailureCategoryCancelled,
			leaseReason:     tasks.TaskAttemptStatusCancelled,
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
	if command.attemptFinish != nil && command.attemptFinish.ID > 0 {
		return finishTaskAttemptTx(ctx, q, command.attemptFinish)
	}
	errorMessage := strings.TrimSpace(command.errorMessage)
	if errorMessage == "" && policy.attemptStatus != tasks.TaskAttemptStatusCompleted {
		errorMessage = "task " + string(command.kind)
	}
	_, err := q.FinishRunningTaskAttemptsByTaskIDs(ctx, dbsqlc.FinishRunningTaskAttemptsByTaskIDsParams{
		Status:          policy.attemptStatus,
		FailureStage:    terminalCommandFailureStage(command, policy),
		FailureCategory: terminalCommandFailureCategory(command, policy),
		ErrorMsg:        errorMessage,
		FinishedAt:      sql.NullTime{Time: command.finishedAt, Valid: true},
		TaskIds:         []string{command.taskID},
	})
	return err
}

func normalizeTerminalAttemptFinishTx(
	ctx context.Context,
	q *dbsqlc.Queries,
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

	switch command.lease.mode {
	case terminalLeaseReleaseExact:
		return taskAttemptFinishWithLeaseIdentity(
			&finish,
			command.taskID,
			command.lease.accountID,
			command.lease.providerEndpointID,
			command.lease.credentialID,
			command.lease.leaseToken,
		)
	case terminalLeaseReleaseActive:
		lease, err := q.GetActiveTaskLeaseByTaskID(ctx, command.taskID)
		if err != nil {
			return nil, fmt.Errorf("resolve active lease for terminal attempt: %w", err)
		}
		return taskAttemptFinishWithLeaseIdentity(
			&finish,
			command.taskID,
			lease.AccountID,
			lease.ProviderEndpointID,
			lease.CredentialID,
			lease.LeaseToken,
		)
	case terminalLeaseReleaseNone:
		if strings.TrimSpace(finish.TaskID) == "" {
			finish.TaskID = command.taskID
		}
		if err := validateTaskAttemptFinishIdentity(finish); err != nil {
			return nil, err
		}
		return &finish, nil
	default:
		return nil, fmt.Errorf("unsupported task terminal lease release mode %d", command.lease.mode)
	}
}

func releaseTerminalTaskLeaseTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	command terminalTransitionCommand,
	policy terminalTransitionPolicy,
) error {
	switch command.lease.mode {
	case terminalLeaseReleaseNone:
		return nil
	case terminalLeaseReleaseExact:
		return releaseTaskLeaseTx(
			ctx,
			q,
			command.taskID,
			command.lease.accountID,
			command.lease.providerEndpointID,
			command.lease.credentialID,
			command.lease.leaseToken,
			policy.leaseReason,
			command.finishedAt,
		)
	case terminalLeaseReleaseActive:
		return releaseActiveTaskLeaseByTaskIDTx(ctx, q, command.taskID, policy.leaseReason, command.finishedAt)
	default:
		return fmt.Errorf("unsupported task terminal lease release mode %d", command.lease.mode)
	}
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
