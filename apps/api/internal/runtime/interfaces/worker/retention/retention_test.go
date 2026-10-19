package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/platform/riverpolicy"
	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
	purgecontracts "ideaboard/api/internal/tenancy/ports/purge"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type retentionServiceStub struct {
	days  int
	calls int
	stats retentioncontracts.PurgedTaskStats
	err   error
}

func (s *retentionServiceStub) PurgeExpiredTasks(_ context.Context, retentionDays int) (retentioncontracts.PurgedTaskStats, error) {
	s.calls++
	s.days = retentionDays
	return s.stats, s.err
}

func TestTaskRetentionWorkerPurgesWithConfiguredWindow(t *testing.T) {
	service := &retentionServiceStub{stats: retentioncontracts.PurgedTaskStats{TasksDeleted: 5}}
	worker := NewTaskRetentionWorker(service, 7)
	job := &river.Job[taskRetentionScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("PurgeExpiredTasks() calls = %d, want 1", service.calls)
	}
	if service.days != 7 {
		t.Fatalf("retention days = %d, want 7", service.days)
	}
}

func TestTaskRetentionWorkerPropagatesFailure(t *testing.T) {
	wantErr := errors.New("purge unavailable")
	service := &retentionServiceStub{err: wantErr}
	worker := NewTaskRetentionWorker(service, 7)
	job := &river.Job[taskRetentionScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
	}
	if err := worker.Work(t.Context(), job); !errors.Is(err, wantErr) {
		t.Fatalf("Work() error = %v, want %v", err, wantErr)
	}
}

func TestTaskRetentionWorkerRejectsInvalidJob(t *testing.T) {
	worker := NewTaskRetentionWorker(&retentionServiceStub{}, 7)
	if err := worker.Work(t.Context(), nil); err == nil {
		t.Fatal("Work(nil) error = nil, want error")
	}
	if err := NewTaskRetentionWorker(nil, 7).Work(t.Context(), &river.Job[taskRetentionScheduleJobArgs]{
		JobRow: &rivertype.JobRow{},
	}); err == nil {
		t.Fatal("Work() with nil service error = nil, want error")
	}
}

func TestTaskRetentionPeriodicJobs(t *testing.T) {
	if jobs := PeriodicJobs(); len(jobs) != 2 {
		t.Fatalf("periodic job count = %d, want 2 (daily + catch-up)", len(jobs))
	}
}

func TestTaskRetentionDailyScheduleRunsAt0330Shanghai(t *testing.T) {
	schedule := taskRetentionDailySchedule{}
	before := time.Date(2026, 9, 17, 0, 0, 0, 0, retentionLocation)
	if got := schedule.Next(before); got.Hour() != 3 || got.Minute() != 30 || got.Day() != 17 {
		t.Fatalf("Next() before slot = %v, want same day 03:30", got)
	}
	after := time.Date(2026, 9, 17, 4, 0, 0, 0, retentionLocation)
	if got := schedule.Next(after); got.Hour() != 3 || got.Minute() != 30 || got.Day() != 18 {
		t.Fatalf("Next() after slot = %v, want next day 03:30", got)
	}
}

func TestTaskRetentionWorkerPinsTimeoutMatrix(t *testing.T) {
	if got := (&TaskRetentionWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutTaskRetention {
		t.Fatalf("retention timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutTaskRetention)
	}
}

type workspacePurgeServiceStub struct {
	days  int
	calls int
	stats purgecontracts.PurgedWorkspaceStats
	err   error
}

func (s *workspacePurgeServiceStub) PurgeExpiredWorkspaces(_ context.Context, purgeAfterDays int) (purgecontracts.PurgedWorkspaceStats, error) {
	s.calls++
	s.days = purgeAfterDays
	return s.stats, s.err
}

func TestWorkspacePurgeWorkerPurgesWithConfiguredWindow(t *testing.T) {
	service := &workspacePurgeServiceStub{stats: purgecontracts.PurgedWorkspaceStats{WorkspacesDeleted: 2}}
	worker := NewWorkspacePurgeWorker(service, 7)
	job := &river.Job[workspacePurgeScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
	}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("PurgeExpiredWorkspaces() calls = %d, want 1", service.calls)
	}
	if service.days != 7 {
		t.Fatalf("purge window = %d, want 7", service.days)
	}
}

func TestWorkspacePurgeWorkerPropagatesFailure(t *testing.T) {
	wantErr := errors.New("purge unavailable")
	service := &workspacePurgeServiceStub{err: wantErr}
	worker := NewWorkspacePurgeWorker(service, 7)
	job := &river.Job[workspacePurgeScheduleJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
	}
	if err := worker.Work(t.Context(), job); !errors.Is(err, wantErr) {
		t.Fatalf("Work() error = %v, want %v", err, wantErr)
	}
}

func TestWorkspacePurgeWorkerRejectsInvalidJob(t *testing.T) {
	worker := NewWorkspacePurgeWorker(&workspacePurgeServiceStub{}, 7)
	if err := worker.Work(t.Context(), nil); err == nil {
		t.Fatal("Work(nil) error = nil, want error")
	}
	if err := NewWorkspacePurgeWorker(nil, 7).Work(t.Context(), &river.Job[workspacePurgeScheduleJobArgs]{
		JobRow: &rivertype.JobRow{},
	}); err == nil {
		t.Fatal("Work() with nil service error = nil, want error")
	}
}

func TestWorkspacePurgePeriodicJobs(t *testing.T) {
	if jobs := WorkspacePurgePeriodicJobs(); len(jobs) != 2 {
		t.Fatalf("periodic job count = %d, want 2 (daily + catch-up)", len(jobs))
	}
}

func TestWorkspacePurgeWorkerPinsTimeoutMatrix(t *testing.T) {
	if got := (&WorkspacePurgeWorker{}).Timeout(nil); got != riverpolicy.RiverJobTimeoutTaskRetention {
		t.Fatalf("workspace purge timeout = %s, want matrix %s", got, riverpolicy.RiverJobTimeoutTaskRetention)
	}
}

func TestWorkspacePurgeScheduleKind(t *testing.T) {
	if got := (workspacePurgeScheduleJobArgs{}).Kind(); got != "workspace_purge_schedule" {
		t.Fatalf("Kind() = %q, want workspace_purge_schedule", got)
	}
}
