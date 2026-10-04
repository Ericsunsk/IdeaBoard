package reconciliation

import (
	"testing"
	"time"
)

func TestPeriodicJobsRegistersDailyScanAndCatchUp(t *testing.T) {
	jobs := PeriodicJobs()
	if len(jobs) != 2 {
		t.Fatalf("periodic jobs = %d, want daily scan plus startup catch-up", len(jobs))
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
	if fingerprintForDraft(draft) != fingerprintForDraft(draft) {
		t.Fatal("fingerprint is not stable")
	}
	other := draft
	other.FindingType = FindingBanStateMismatch
	if fingerprintForDraft(draft) == fingerprintForDraft(other) {
		t.Fatal("different finding types share a fingerprint")
	}
	if len(fingerprintForDraft(draft)) != 64 {
		t.Fatalf("fingerprint = %q, want 64 hex chars", fingerprintForDraft(draft))
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
