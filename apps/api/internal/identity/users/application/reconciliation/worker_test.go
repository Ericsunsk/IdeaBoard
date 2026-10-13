package reconciliation

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

func TestPeriodicJobsRegistersDailyScanAndCatchUp(t *testing.T) {
	jobs := PeriodicJobs()
	if len(jobs) != 3 {
		t.Fatalf("periodic jobs = %d, want daily scan plus startup catch-up plus auth sync sweep", len(jobs))
	}
	for _, job := range jobs {
		if job == nil {
			t.Fatal("periodic job is nil")
		}
	}
}

func TestDailyScheduleRunsAtThreeAMShanghai(t *testing.T) {
	schedule := dailySchedule{}
	current := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC) // 18:00 Shanghai
	next := schedule.Next(current)
	want := time.Date(2026, 10, 5, 3, 0, 0, 0, scheduleLocation)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
	slot := latestScheduleSlot(current)
	wantSlot := time.Date(2026, 10, 4, 3, 0, 0, 0, scheduleLocation)
	if !slot.Equal(wantSlot) {
		t.Fatalf("slot = %v, want %v", slot, wantSlot)
	}
}

func TestFingerprintIsStablePerDriftSubject(t *testing.T) {
	draft := findingDraft{
		FindingType: FindingRoleMismatch, UserID: 7,
		SubjectType: "user", SubjectID: "public-id",
	}
	fingerprint := fingerprintForDraft(draft)
	updated := draft
	updated.Severity = "critical"
	updated.Details = map[string]any{"observed_role": "admin"}
	if fingerprint != fingerprintForDraft(updated) {
		t.Fatal("finding details changed the subject fingerprint")
	}
	other := draft
	other.FindingType = FindingBanStateMismatch
	if fingerprint == fingerprintForDraft(other) {
		t.Fatal("different finding types share a fingerprint")
	}
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint = %q, want 64 hex chars", fingerprint)
	}
}

func TestParseScheduleMode(t *testing.T) {
	if overdue, err := parseScheduleMode(scheduleModeDaily); err != nil || overdue {
		t.Fatalf("daily mode = %v, %v, want onlyIfOverdue=false", overdue, err)
	}
	if overdue, err := parseScheduleMode(scheduleModeCatchUp); err != nil || !overdue {
		t.Fatalf("catch-up mode = %v, %v, want onlyIfOverdue=true", overdue, err)
	}
	if _, err := parseScheduleMode("hourly"); err == nil {
		t.Fatal("unknown schedule mode succeeded, want error")
	}
}

type fakeSyncSweepScheduleService struct {
	scheduleCalls int
	sweepCalls    int
	sweepResult   int
	sweepErr      error
}

func (f *fakeSyncSweepScheduleService) ScheduleReconciliation(_ context.Context, _ RunDispatcher, _ time.Time, _ bool) (*Run, error) {
	f.scheduleCalls++
	return nil, nil
}

func (f *fakeSyncSweepScheduleService) SweepAuthSync(_ context.Context) (int, error) {
	f.sweepCalls++
	return f.sweepResult, f.sweepErr
}

func TestScheduleWorkerSyncSweepModeConverges(t *testing.T) {
	service := &fakeSyncSweepScheduleService{sweepResult: 3}
	worker := NewScheduleWorker(service, &stubSyncSweepDispatcher{})
	if err := worker.Work(context.Background(), &river.Job[scheduleArgs]{Args: scheduleArgs{Mode: scheduleModeSyncSweep}}); err != nil {
		t.Fatalf("sync sweep work: %v", err)
	}
	if service.sweepCalls != 1 || service.scheduleCalls != 0 {
		t.Fatalf("sweep calls = %d schedule calls = %d, want 1/0", service.sweepCalls, service.scheduleCalls)
	}
}

type stubSyncSweepDispatcher struct{}

func (stubSyncSweepDispatcher) DispatchReconciliationRun(_ context.Context, _ pgx.Tx, _ int64) error {
	return nil
}
