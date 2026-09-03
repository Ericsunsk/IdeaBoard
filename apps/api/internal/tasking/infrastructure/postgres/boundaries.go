package postgres

// Capability interfaces keep callers dependent on the smallest taskstore
// surface they need. Repository itself implements every capability; returning
// it as an interface avoids duplicate forwarding wrappers while preserving a
// single source of truth for transactions, idempotency and lease/CAS logic.

import (
	"context"
	"time"

	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"

	tasks "ideaboard/api/internal/tasking/domain"
	submissioncontracts "ideaboard/api/internal/tasking/ports/submission"

	"github.com/jackc/pgx/v5"
)

type SubmissionRepository interface {
	CreateWithPGXTx(context.Context, pgx.Tx, *tasks.Task) (bool, error)
	LockAndFindSubmissionByClientActionWithPGXTx(context.Context, pgx.Tx, submissioncontracts.Lookup) (tasks.TaskSubmissionResult, bool, error)
	PersistSubmissionClaim(context.Context, pgx.Tx, submissioncontracts.Claim) error
	ClaimTaskInputMediaRef(context.Context, pgx.Tx, tasks.MediaRefClaim) error
}

type TaskReader interface {
	GetByTaskID(context.Context, string) (*tasks.Task, error)
}

type TaskCancellationRepository interface {
	MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error)
}

type RuntimeRepository interface {
	ConvergeTask(context.Context, runtimecontracts.TaskConvergenceIntent) (int64, error)
	TaskReader
	GetActiveTaskLeaseByTaskID(context.Context, string) (*runtimecontracts.ActiveTaskLease, error)
	PatchExecutionMetrics(context.Context, tasks.TaskExecutionMetricsPatch) error
	PatchExecutionSnapshot(context.Context, tasks.TaskExecutionSnapshot) (int64, error)
	CreateTaskAttempt(context.Context, tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error)
	FinishTaskAttempt(context.Context, tasks.TaskAttemptFinish) (int64, error)
	TaskCancellationRepository
}

type QueryRepository interface {
	TaskReader
	GetByTaskIDAndUserID(context.Context, string, int64) (*tasks.Task, error)
	List(context.Context, int64, int64, string, string, int, int) ([]*tasks.Task, tasks.TaskListCounts, error)
	GetByUserID(context.Context, int64, int, int) ([]*tasks.Task, int64, error)
	ListOutputsByTaskIDs(context.Context, []string) (map[string][]tasks.TaskOutput, error)
	TaskCancellationRepository
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
	ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.DailyStatAggregate, error)
	ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error)
	ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error)
	ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventTrendRow, error)
	ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventLatencySample, error)
	ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error)
	ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error)
	BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (statisticscontracts.BackfillDailyStatsResult, error)
}

func (r *Repository) Submission() SubmissionRepository { return r }
func (r *Repository) Runtime() RuntimeRepository       { return r }
func (r *Repository) Queries() QueryRepository         { return r }
func (r *Repository) Billing() BillingRepository       { return r }
func (r *Repository) Trace() TraceRepository           { return r }
func (r *Repository) Statistics() StatisticsRepository { return r }

var _ SubmissionRepository = (*Repository)(nil)
var _ RuntimeRepository = (*Repository)(nil)
var _ QueryRepository = (*Repository)(nil)
var _ BillingRepository = (*Repository)(nil)
var _ TraceRepository = (*Repository)(nil)
var _ StatisticsRepository = (*Repository)(nil)
