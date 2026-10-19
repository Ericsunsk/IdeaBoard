package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/platform/riverpolicy"
	purgecontracts "ideaboard/api/internal/tenancy/ports/purge"

	"github.com/riverqueue/river"
)

const (
	// QueueName is a single-worker queue: purge runs are serial by design.
	// It aliases the Platform-owned policy constant so inserts track the
	// matrix with no copied literal.
	QueueName = riverpolicy.RiverQueueTaskRetention
	// The purge Timeout lives in the River timeout matrix
	// (riverpolicy.RiverJobTimeoutTaskRetention).
)

// retentionLocation matches the other daily maintenance jobs so operators
// see one maintenance window in Asia/Shanghai time.
var retentionLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type retentionDailySchedule struct{}

func (retentionDailySchedule) Next(current time.Time) time.Time {
	local := current.In(retentionLocation)
	next := time.Date(local.Year(), local.Month(), local.Day(), 3, 30, 0, 0, retentionLocation)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// workspacePurgeScheduleJobArgs is the River kind for the archived
// workspace purge. Expired archives converge once a day at 03:30
// Asia/Shanghai on the serial maintenance queue, never concurrently.
type workspacePurgeScheduleJobArgs struct{}

func (workspacePurgeScheduleJobArgs) Kind() string { return "workspace_purge_schedule" }

type workspacePurgeService interface {
	PurgeExpiredWorkspaces(ctx context.Context, purgeAfterDays int) (purgecontracts.PurgedWorkspaceStats, error)
}

// WorkspacePurgeWorker hard-deletes workspaces archived longer ago than
// the configured window. Runs are serial by queue design; one poisoned
// workspace fails the job (River retry) without rolling back already
// committed workspace deletes.
type WorkspacePurgeWorker struct {
	river.WorkerDefaults[workspacePurgeScheduleJobArgs]
	service        workspacePurgeService
	purgeAfterDays int
}

// NewWorkspacePurgeWorker builds a WorkspacePurgeWorker over service.
func NewWorkspacePurgeWorker(service workspacePurgeService, purgeAfterDays int) *WorkspacePurgeWorker {
	return &WorkspacePurgeWorker{service: service, purgeAfterDays: purgeAfterDays}
}

func (w *WorkspacePurgeWorker) Work(ctx context.Context, job *river.Job[workspacePurgeScheduleJobArgs]) error {
	if w == nil || w.service == nil || job == nil {
		return fmt.Errorf("invalid workspace purge job")
	}
	result, err := w.service.PurgeExpiredWorkspaces(ctx, w.purgeAfterDays)
	if err != nil {
		return err
	}
	slog.Info("workspace purge completed",
		"purge_after_days", w.purgeAfterDays,
		"workspaces_deleted", result.WorkspacesDeleted,
		"storage_objects_deleted", result.StorageObjectsDeleted,
		"storage_delete_failures", result.StorageDeleteFailures,
	)
	return nil
}

func (*WorkspacePurgeWorker) Timeout(*river.Job[workspacePurgeScheduleJobArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutTaskRetention
}

// WorkspacePurgePeriodicJobs returns the daily + catch-up periodic jobs
// for the workspace purge schedule.
func WorkspacePurgePeriodicJobs() []*river.PeriodicJob {
	insertOpts := func() *river.InsertOpts {
		return &river.InsertOpts{Queue: QueueName, MaxAttempts: riverpolicy.RiverMaxAttemptsTaskRetention, Priority: 4}
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			retentionDailySchedule{},
			func() (river.JobArgs, *river.InsertOpts) {
				return workspacePurgeScheduleJobArgs{}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "workspace-purge-daily", RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NeverSchedule(),
			func() (river.JobArgs, *river.InsertOpts) {
				return workspacePurgeScheduleJobArgs{}, insertOpts()
			},
			&river.PeriodicJobOpts{ID: "workspace-purge-catch-up", RunOnStart: true},
		),
	}
}
