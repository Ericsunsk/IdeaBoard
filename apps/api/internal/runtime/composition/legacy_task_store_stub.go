package composition

// legacyTaskStore is the #816 (Phase 3b-07) fail-closed stand-in for the
// deleted legacy taskstore.Repository.
//
// The legacy tasks.sql queries (CreateTaskIdempotent,
// ClaimAndStartExecutionAttempt, ListAdminTaskAttemptsByTaskIDs,
// ListOutputsByTaskIDs) were removed with the tasks table rename
// (tasks_v2 -> tasks, now workspace-scoped with public_id), so the postgres
// adapter under internal/tasking/infrastructure/postgres was deleted.
// Every method below fails closed with errLegacyTaskStoreRemoved, except
// ListRunningTasksForStartupRecovery which reports no recoverable tasks so
// process startup proceeds (there is no legacy running row left to
// converge). The workspace-scoped store (Phase 4a) restores real
// implementations behind the same narrow consumer seams.

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/runtime/application/orchestration"
	taskexecution "ideaboard/api/internal/tasking/application/execution"
	taskretention "ideaboard/api/internal/tasking/application/retention"
	taskstatistics "ideaboard/api/internal/tasking/application/statistics"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	taskservice "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	submissioncontracts "ideaboard/api/internal/tasking/ports/submission"

	"github.com/jackc/pgx/v5"
)

var errLegacyTaskStoreRemoved = errors.New("legacy task store removed (#816 Phase 3b-07): workspace-scoped store lands in Phase 4a")

type legacyTaskStore struct{}

var _ tasksubmission.SubmissionTaskStore = (*legacyTaskStore)(nil)
var _ taskexecution.TaskStore = (*legacyTaskStore)(nil)
var _ taskservice.TaskStore = (*legacyTaskStore)(nil)
var _ taskservice.AdminBillingHistoryStore = (*legacyTaskStore)(nil)
var _ taskservice.AdminTaskTraceStore = (*legacyTaskStore)(nil)
var _ taskservice.AdminTaskListReadStore = (*legacyTaskStore)(nil)
var _ taskstatistics.Store = (*legacyTaskStore)(nil)
var _ taskretention.Store = (*legacyTaskStore)(nil)
var _ orchestration.StartupRecoveryTaskStore = (*legacyTaskStore)(nil)

func (s *legacyTaskStore) CreateWithPGXTx(context.Context, pgx.Tx, *tasks.Task) (bool, error) {
	return false, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) LockAndFindSubmissionByClientActionWithPGXTx(context.Context, pgx.Tx, submissioncontracts.Lookup) (tasks.TaskSubmissionResult, bool, error) {
	return tasks.TaskSubmissionResult{}, false, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) PersistSubmissionClaim(context.Context, pgx.Tx, submissioncontracts.Claim) error {
	return errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ClaimTaskInputMediaRef(context.Context, pgx.Tx, tasks.MediaRefClaim) error {
	return errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ConvergeTask(context.Context, runtimecontracts.TaskConvergenceIntent) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) GetActiveExecutionAttemptByTaskID(context.Context, string) (*runtimecontracts.ActiveTaskExecution, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ClaimAndStartExecutionAttempt(context.Context, tasks.ExecutionAttemptStart) (int32, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) PatchExecutionMetrics(context.Context, tasks.TaskExecutionMetricsPatch) error {
	return errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) PatchExecutionSnapshot(context.Context, tasks.TaskExecutionSnapshot) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) FinalizeExecutionAttempt(context.Context, tasks.ExecutionAttemptFinish) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) GetByTaskIDAndUserID(context.Context, string, int64) (*tasks.Task, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) List(context.Context, int64, int64, string, string, int, int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	return nil, tasks.TaskListCounts{}, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) GetByUserID(context.Context, int64, int, int) ([]*tasks.Task, int64, error) {
	return nil, 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListOutputsByTaskIDs(context.Context, []string) (map[string][]tasks.TaskOutput, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListAdminBillingSnapshots(context.Context, []string) (map[string]tasks.AdminBillingSnapshot, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListAdminTaskAttempts(context.Context, []string) (map[string][]tasks.AdminTaskAttempt, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListAdminTaskLatestExecutions(context.Context, []string) (map[string]tasks.AdminTaskExecutionSnapshot, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) CountByStatus(context.Context, string) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) CountAllTaskCalls(context.Context) (int64, error) {
	return 0, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListDailyCaptureCreditsByRange(context.Context, time.Time, time.Time, *time.Location) (map[string]int64, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	return nil, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	return statisticscontracts.BackfillDailyStatsResult{}, errLegacyTaskStoreRemoved
}

func (s *legacyTaskStore) PurgeExpiredTerminalTasks(context.Context, time.Time, int32) (retentioncontracts.PurgedTaskStats, error) {
	return retentioncontracts.PurgedTaskStats{}, errLegacyTaskStoreRemoved
}

// ListRunningTasksForStartupRecovery reports no recoverable tasks: the
// legacy running rows are gone with the tasks table rename, so startup
// recovery is a no-op until Phase 4a re-supplies the workspace-scoped store.
func (s *legacyTaskStore) ListRunningTasksForStartupRecovery(context.Context) ([]orchestration.StartupRecoveredTask, error) {
	return nil, nil
}
