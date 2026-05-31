package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"api/internal/accounts"
)

func TestParseRepairTaskLeasesArgsDefaults(t *testing.T) {
	opts, err := parseRepairTaskLeasesArgs([]string{"task-leases"})
	if err != nil {
		t.Fatalf("parseRepairTaskLeasesArgs returned error: %v", err)
	}
	if !opts.DryRun {
		t.Fatal("DryRun should default to true")
	}
	if opts.To != "" {
		t.Fatalf("To=%q, want empty", opts.To)
	}
	if opts.Yes {
		t.Fatal("Yes should default to false")
	}
	if opts.Limit != defaultRepairLimit {
		t.Fatalf("Limit=%d, want %d", opts.Limit, defaultRepairLimit)
	}
}

func TestParseRepairTaskLeasesArgsMutationMode(t *testing.T) {
	opts, err := parseRepairTaskLeasesArgs([]string{
		"task-leases", "--to=Pending", "--yes", "--limit=42",
	})
	if err != nil {
		t.Fatalf("parseRepairTaskLeasesArgs returned error: %v", err)
	}
	if opts.DryRun {
		t.Fatal("DryRun should be false")
	}
	if opts.To != "pending" {
		t.Fatalf("To=%q, want pending", opts.To)
	}
	if !opts.Yes {
		t.Fatal("Yes should be true")
	}
	if opts.Limit != 42 {
		t.Fatalf("Limit=%d, want 42", opts.Limit)
	}
}

func TestParseRepairTaskLeasesArgsExplicitDryRunWins(t *testing.T) {
	opts, err := parseRepairTaskLeasesArgs([]string{
		"task-leases", "--dry-run", "--to=pending", "--yes",
	})
	if err != nil {
		t.Fatalf("parseRepairTaskLeasesArgs returned error: %v", err)
	}
	if !opts.DryRun {
		t.Fatal("DryRun should remain true when explicitly set")
	}
}

func TestTaskLeaseStartupRepairHint(t *testing.T) {
	err := taskLeaseStartupRepairHint(errors.New("running task lease missing token: task_id=x account_id=1"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "api preflight task-leases") {
		t.Fatalf("missing preflight hint in %q", msg)
	}
	if !strings.Contains(msg, "api repair task-leases --to=pending --yes") {
		t.Fatalf("missing repair hint in %q", msg)
	}
}

func TestRunTaskLeasePreflightOK(t *testing.T) {
	repo := &fakeTaskLeasePreflightRepo{count: 0}
	if err := runTaskLeasePreflight(context.Background(), repo, 5); err != nil {
		t.Fatalf("runTaskLeasePreflight returned error: %v", err)
	}
}

func TestRunTaskLeasePreflightFindings(t *testing.T) {
	repo := &fakeTaskLeasePreflightRepo{
		count: 2,
		list: []accounts.RunningTaskLease{
			{TaskID: "task-a", AccountID: 11},
			{TaskID: "task-b", AccountID: 22},
		},
	}
	err := runTaskLeasePreflight(context.Background(), repo, 2)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if !strings.Contains(err.Error(), "found 2 running task(s) missing lease_token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportTaskLeasePreflightFindingsDoesNotFailDryRun(t *testing.T) {
	repo := &fakeTaskLeasePreflightRepo{
		count: 1,
		list:  []accounts.RunningTaskLease{{TaskID: "task-a", AccountID: 11}},
	}
	report, err := reportTaskLeasePreflight(context.Background(), repo, 5)
	if err != nil {
		t.Fatalf("reportTaskLeasePreflight returned error: %v", err)
	}
	if report.Count != 1 || len(report.Sample) != 1 {
		t.Fatalf("report=%+v, want count=1 sample=1", report)
	}
}

type fakeTaskLeasePreflightRepo struct {
	count int64
	list  []accounts.RunningTaskLease
}

func (f *fakeTaskLeasePreflightRepo) CountRunningTasksMissingLeaseToken(context.Context) (int64, error) {
	return f.count, nil
}

func (f *fakeTaskLeasePreflightRepo) ListRunningTasksMissingLeaseToken(context.Context, int) ([]accounts.RunningTaskLease, error) {
	return f.list, nil
}

func (f *fakeTaskLeasePreflightRepo) ResetRunningTasksMissingLeaseTokenToPending(context.Context, int) (int64, error) {
	return 0, nil
}
