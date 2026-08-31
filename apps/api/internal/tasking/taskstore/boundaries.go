package taskstore

// This file defines the taskstore's domain boundaries.  Repository remains the
// single database-backed facade, while callers depend on the smallest capability
// surface they need.  The boundary objects intentionally delegate to the same
// repository so transaction, lease/CAS and idempotency invariants have one fact
// source.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
)

type SubmissionRepository interface {
	CreateWithPGXTx(context.Context, pgx.Tx, *tasks.Task) (bool, error)
	LockAndFindSubmissionByClientActionWithPGXTx(context.Context, pgx.Tx, int64, string, taskbilling.Request) (tasks.TaskSubmissionResult, bool, error)
	ClaimTaskInputMediaRef(context.Context, pgx.Tx, string, int64, int64, string, string, int) error
}

type RuntimeRepository interface {
	ConvergeTask(context.Context, TaskConvergenceIntent) (int64, error)
	GetByTaskID(context.Context, string) (*tasks.Task, error)
	GetActiveTaskLeaseByTaskID(context.Context, string) (*ActiveTaskLease, error)
	PatchExecutionMetrics(context.Context, tasks.TaskExecutionMetricsPatch) error
	PatchExecutionSnapshot(context.Context, tasks.TaskExecutionSnapshot) (int64, error)
	CreateTaskAttempt(context.Context, tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error)
	FinishTaskAttempt(context.Context, tasks.TaskAttemptFinish) (int64, error)
	MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error)
}

type QueryRepository interface {
	GetByTaskID(context.Context, string) (*tasks.Task, error)
	GetByTaskIDAndUserID(context.Context, string, int64) (*tasks.Task, error)
	List(context.Context, int64, int64, string, string, int, int) ([]*tasks.Task, tasks.TaskListCounts, error)
	GetByUserID(context.Context, int64, int, int) ([]*tasks.Task, int64, error)
	ListOutputsByTaskIDs(context.Context, []string) (map[string][]tasks.TaskOutput, error)
	MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error)
	ListTaskIDsByAccountIDAndStatuses(context.Context, int64, ...string) ([]string, error)
}

type BillingRepository interface {
	ListAdminBillingSnapshots(context.Context, []string) (map[string]tasks.AdminBillingSnapshot, error)
}

type TraceRepository interface {
	ListAdminTaskAttempts(context.Context, []string) (map[string][]tasks.AdminTaskAttempt, error)
	ListAdminTaskLatestExecutions(context.Context, []string) (map[string]tasks.AdminTaskExecutionSnapshot, error)
}

type StatisticsRepository interface {
	CountByStatus(context.Context, string) (int64, error)
	CountAllTaskCalls(context.Context) (int64, error)
	ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error)
	ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]DailyStatAggregate, error)
	ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]ExecutionBottleneckAggregate, error)
	ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]ExecutionFailureCategoryAggregate, error)
	ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]StatEventTrendRow, error)
	ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]StatEventLatencySample, error)
	ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]MonitorExecutionPressureAggregate, error)
	ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]MonitorGoldenSignalSample, error)
	BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (BackfillDailyStatsResult, error)
}

type submissionBoundary struct{ repository *Repository }
type runtimeBoundary struct{ repository *Repository }
type queryBoundary struct{ repository *Repository }
type billingBoundary struct{ repository *Repository }
type traceBoundary struct{ repository *Repository }
type statisticsBoundary struct{ repository *Repository }

func (b submissionBoundary) CreateWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error) {
	return b.repository.CreateWithPGXTx(ctx, tx, task)
}
func (b submissionBoundary) LockAndFindSubmissionByClientActionWithPGXTx(ctx context.Context, tx pgx.Tx, userID int64, actionID string, request taskbilling.Request) (tasks.TaskSubmissionResult, bool, error) {
	return b.repository.LockAndFindSubmissionByClientActionWithPGXTx(ctx, tx, userID, actionID, request)
}
func (b submissionBoundary) ClaimTaskInputMediaRef(ctx context.Context, tx pgx.Tx, taskID string, userID, assetID int64, refID, role string, index int) error {
	return b.repository.ClaimTaskInputMediaRef(ctx, tx, taskID, userID, assetID, refID, role, index)
}

func (b runtimeBoundary) ConvergeTask(ctx context.Context, i TaskConvergenceIntent) (int64, error) {
	return b.repository.ConvergeTask(ctx, i)
}
func (b runtimeBoundary) GetByTaskID(ctx context.Context, id string) (*tasks.Task, error) {
	return b.repository.GetByTaskID(ctx, id)
}
func (b runtimeBoundary) GetActiveTaskLeaseByTaskID(ctx context.Context, id string) (*ActiveTaskLease, error) {
	return b.repository.GetActiveTaskLeaseByTaskID(ctx, id)
}
func (b runtimeBoundary) PatchExecutionMetrics(ctx context.Context, p tasks.TaskExecutionMetricsPatch) error {
	return b.repository.PatchExecutionMetrics(ctx, p)
}
func (b runtimeBoundary) PatchExecutionSnapshot(ctx context.Context, s tasks.TaskExecutionSnapshot) (int64, error) {
	return b.repository.PatchExecutionSnapshot(ctx, s)
}
func (b runtimeBoundary) CreateTaskAttempt(ctx context.Context, s tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	return b.repository.CreateTaskAttempt(ctx, s)
}
func (b runtimeBoundary) FinishTaskAttempt(ctx context.Context, f tasks.TaskAttemptFinish) (int64, error) {
	return b.repository.FinishTaskAttempt(ctx, f)
}
func (b runtimeBoundary) MarkTaskCancelled(ctx context.Context, s tasks.TaskCancellationSnapshot, f *tasks.TaskAttemptFinish) (int64, error) {
	return b.repository.MarkTaskCancelled(ctx, s, f)
}

func (b queryBoundary) GetByTaskID(ctx context.Context, id string) (*tasks.Task, error) {
	return b.repository.GetByTaskID(ctx, id)
}
func (b queryBoundary) GetByTaskIDAndUserID(ctx context.Context, id string, userID int64) (*tasks.Task, error) {
	return b.repository.GetByTaskIDAndUserID(ctx, id, userID)
}
func (b queryBoundary) List(ctx context.Context, userID, accountID int64, status, keyword string, page, size int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	return b.repository.List(ctx, userID, accountID, status, keyword, page, size)
}
func (b queryBoundary) GetByUserID(ctx context.Context, userID int64, page, size int) ([]*tasks.Task, int64, error) {
	return b.repository.GetByUserID(ctx, userID, page, size)
}
func (b queryBoundary) ListOutputsByTaskIDs(ctx context.Context, ids []string) (map[string][]tasks.TaskOutput, error) {
	return b.repository.ListOutputsByTaskIDs(ctx, ids)
}
func (b queryBoundary) MarkTaskCancelled(ctx context.Context, s tasks.TaskCancellationSnapshot, f *tasks.TaskAttemptFinish) (int64, error) {
	return b.repository.MarkTaskCancelled(ctx, s, f)
}
func (b queryBoundary) ListTaskIDsByAccountIDAndStatuses(ctx context.Context, id int64, statuses ...string) ([]string, error) {
	return b.repository.ListTaskIDsByAccountIDAndStatuses(ctx, id, statuses...)
}

func (b billingBoundary) ListAdminBillingSnapshots(ctx context.Context, ids []string) (map[string]tasks.AdminBillingSnapshot, error) {
	return b.repository.ListAdminBillingSnapshots(ctx, ids)
}
func (b traceBoundary) ListAdminTaskAttempts(ctx context.Context, ids []string) (map[string][]tasks.AdminTaskAttempt, error) {
	return b.repository.ListAdminTaskAttempts(ctx, ids)
}
func (b traceBoundary) ListAdminTaskLatestExecutions(ctx context.Context, ids []string) (map[string]tasks.AdminTaskExecutionSnapshot, error) {
	return b.repository.ListAdminTaskLatestExecutions(ctx, ids)
}

func (b statisticsBoundary) CountByStatus(ctx context.Context, status string) (int64, error) {
	return b.repository.CountByStatus(ctx, status)
}
func (b statisticsBoundary) CountAllTaskCalls(ctx context.Context) (int64, error) {
	return b.repository.CountAllTaskCalls(ctx)
}
func (b statisticsBoundary) ListBackfilledDatesByRange(ctx context.Context, s, e time.Time) ([]time.Time, error) {
	return b.repository.ListBackfilledDatesByRange(ctx, s, e)
}
func (b statisticsBoundary) ListDailyStatsByDateRange(ctx context.Context, s, e time.Time) ([]DailyStatAggregate, error) {
	return b.repository.ListDailyStatsByDateRange(ctx, s, e)
}
func (b statisticsBoundary) ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, s, e time.Time) ([]ExecutionBottleneckAggregate, error) {
	return b.repository.ListExecutionBottleneckStatsByCreatedRange(ctx, s, e)
}
func (b statisticsBoundary) ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, s, e time.Time) ([]ExecutionFailureCategoryAggregate, error) {
	return b.repository.ListExecutionFailureCategoryStatsByCreatedRange(ctx, s, e)
}
func (b statisticsBoundary) ListStatEventsByOccurredRange(ctx context.Context, s, e time.Time) ([]StatEventTrendRow, error) {
	return b.repository.ListStatEventsByOccurredRange(ctx, s, e)
}
func (b statisticsBoundary) ListStatEventLatencySamplesByDateRange(ctx context.Context, s, e time.Time) ([]StatEventLatencySample, error) {
	return b.repository.ListStatEventLatencySamplesByDateRange(ctx, s, e)
}
func (b statisticsBoundary) ListMonitorExecutionPressureByRange(ctx context.Context, s, e time.Time) ([]MonitorExecutionPressureAggregate, error) {
	return b.repository.ListMonitorExecutionPressureByRange(ctx, s, e)
}
func (b statisticsBoundary) ListMonitorGoldenSignalSamplesByRange(ctx context.Context, s, e time.Time) ([]MonitorGoldenSignalSample, error) {
	return b.repository.ListMonitorGoldenSignalSamplesByRange(ctx, s, e)
}
func (b statisticsBoundary) BackfillDailyStatsByCreatedRange(ctx context.Context, s, e, d time.Time, mark bool) (BackfillDailyStatsResult, error) {
	return b.repository.BackfillDailyStatsByCreatedRange(ctx, s, e, d, mark)
}

// The returned interfaces are capability-specific seams used by tasking
// services. They prevent new code from depending on the full repository.
func (r *Repository) Submission() SubmissionRepository { return submissionBoundary{repository: r} }
func (r *Repository) Runtime() RuntimeRepository       { return runtimeBoundary{repository: r} }
func (r *Repository) Queries() QueryRepository         { return queryBoundary{repository: r} }
func (r *Repository) Billing() BillingRepository       { return billingBoundary{repository: r} }
func (r *Repository) Trace() TraceRepository           { return traceBoundary{repository: r} }
func (r *Repository) Statistics() StatisticsRepository { return statisticsBoundary{repository: r} }

var _ SubmissionRepository = submissionBoundary{}
var _ RuntimeRepository = runtimeBoundary{}
var _ QueryRepository = queryBoundary{}
var _ BillingRepository = billingBoundary{}
var _ TraceRepository = traceBoundary{}
var _ StatisticsRepository = statisticsBoundary{}
