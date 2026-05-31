package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

	"api/internal/accounts"
	"api/internal/platform/config"
	"api/internal/platform/database"
)

const (
	defaultPreflightSampleLimit = 5
	defaultRepairLimit          = 10000
)

type taskLeasePreflightRepository interface {
	CountRunningTasksMissingLeaseToken(ctx context.Context) (int64, error)
	ListRunningTasksMissingLeaseToken(ctx context.Context, limit int) ([]accounts.RunningTaskLease, error)
	ResetRunningTasksMissingLeaseTokenToPending(ctx context.Context, limit int) (int64, error)
}

type repairTaskLeasesOptions struct {
	DryRun bool
	To     string
	Yes    bool
	Limit  int
}

func runPreflight(args []string) {
	if len(args) == 0 || args[0] != "task-leases" {
		log.Fatalf("Usage: api preflight task-leases")
	}
	if len(args) > 1 {
		log.Fatalf("Usage: api preflight task-leases")
	}

	cfg := config.Load()
	configureLogging(cfg.App.LogFile)

	taskRepo, err := newTaskLeasePreflightTaskRepo(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := runTaskLeasePreflight(context.Background(), taskRepo, defaultPreflightSampleLimit); err != nil {
		log.Fatal(err)
	}
}

func runRepair(args []string) {
	opts, err := parseRepairTaskLeasesArgs(args)
	if err != nil {
		log.Fatal(err)
	}
	if opts.Limit <= 0 {
		log.Fatal("repair limit must be greater than 0")
	}
	if !opts.DryRun && (!opts.Yes || opts.To != "pending") {
		log.Fatal("Refusing to mutate. Use: api repair task-leases --to=pending --yes")
	}

	cfg := config.Load()
	configureLogging(cfg.App.LogFile)

	taskRepo, err := newTaskLeasePreflightTaskRepo(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	if opts.DryRun {
		log.Printf("[Repair] task-leases dry-run mode: no data will be changed")
		report, err := reportTaskLeasePreflight(ctx, taskRepo, defaultPreflightSampleLimit)
		if err != nil {
			log.Fatal(err)
		}
		wouldReset := report.Count
		if int64(opts.Limit) < wouldReset {
			wouldReset = int64(opts.Limit)
		}
		log.Printf("[Repair] task-leases dry-run completed: eligible_missing=%d would_reset_to_pending=%d limit=%d", report.Count, wouldReset, opts.Limit)
		return
	}

	affected, err := taskRepo.ResetRunningTasksMissingLeaseTokenToPending(ctx, opts.Limit)
	if err != nil {
		log.Fatal(fmt.Errorf("repair failed: %w", err))
	}
	log.Printf("[Repair] task-leases completed: reset_to_pending=%d limit=%d", affected, opts.Limit)
}

func parseRepairTaskLeasesArgs(args []string) (repairTaskLeasesOptions, error) {
	opts := repairTaskLeasesOptions{
		DryRun: true,
		Limit:  defaultRepairLimit,
	}
	if len(args) == 0 || args[0] != "task-leases" {
		return opts, errors.New("Usage: api repair task-leases [--dry-run] [--to=pending --yes] [--limit=N]")
	}
	fs := flag.NewFlagSet("repair task-leases", flag.ContinueOnError)
	fs.SetOutput(log.Writer())
	fs.BoolVar(&opts.DryRun, "dry-run", true, "preview only; no mutation")
	fs.StringVar(&opts.To, "to", "", "target status for mutation (supported: pending)")
	fs.BoolVar(&opts.Yes, "yes", false, "confirm mutation")
	fs.IntVar(&opts.Limit, "limit", defaultRepairLimit, "max rows to update")

	if err := fs.Parse(args[1:]); err != nil {
		return opts, err
	}
	if len(fs.Args()) > 0 {
		return opts, fmt.Errorf("unknown arguments: %s", strings.Join(fs.Args(), " "))
	}
	opts.To = strings.ToLower(strings.TrimSpace(opts.To))
	dryRunExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "dry-run" {
			dryRunExplicit = true
		}
	})
	if !dryRunExplicit && opts.Yes && opts.To == "pending" {
		opts.DryRun = false
	}
	return opts, nil
}

type taskLeasePreflightReport struct {
	Count  int64
	Sample []accounts.RunningTaskLease
}

func runTaskLeasePreflight(ctx context.Context, repo taskLeasePreflightRepository, sampleLimit int) error {
	report, err := reportTaskLeasePreflight(ctx, repo, sampleLimit)
	if err != nil {
		return err
	}
	if report.Count == 0 {
		return nil
	}
	return fmt.Errorf("preflight failed: found %d running task(s) missing lease_token; run `api repair task-leases --dry-run` then `api repair task-leases --to=pending --yes`", report.Count)
}

func reportTaskLeasePreflight(ctx context.Context, repo taskLeasePreflightRepository, sampleLimit int) (taskLeasePreflightReport, error) {
	var report taskLeasePreflightReport
	count, err := repo.CountRunningTasksMissingLeaseToken(ctx)
	if err != nil {
		return report, fmt.Errorf("preflight count failed: %w", err)
	}
	report.Count = count
	if count == 0 {
		log.Printf("[Preflight] OK: running tasks with missing lease_token = 0")
		return report, nil
	}
	if sampleLimit <= 0 {
		sampleLimit = defaultPreflightSampleLimit
	}
	sample, err := repo.ListRunningTasksMissingLeaseToken(ctx, sampleLimit)
	if err != nil {
		return report, fmt.Errorf("preflight sample failed: %w", err)
	}
	report.Sample = sample
	log.Printf("[Preflight] Found running tasks missing lease_token: count=%d sample=%d", count, len(sample))
	for i, row := range sample {
		log.Printf("[Preflight] sample[%d] task_id=%s account_id=%d", i, row.TaskID, row.AccountID)
	}
	return report, nil
}

func newTaskLeasePreflightTaskRepo(cfg *config.Config) (taskLeasePreflightRepository, error) {
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	repo := accounts.NewTaskRepository(db)
	casted, ok := any(repo).(taskLeasePreflightRepository)
	if !ok {
		return nil, errors.New("task repository does not implement task-lease preflight methods")
	}
	return casted, nil
}

func taskLeaseStartupRepairHint(reconcileErr error) error {
	if reconcileErr == nil {
		return nil
	}
	msg := strings.ToLower(reconcileErr.Error())
	if !strings.Contains(msg, "running task lease missing token") {
		return reconcileErr
	}
	return fmt.Errorf("%w\n[Main] Startup blocked: found running task(s) missing lease_token.\n[Main] Run preflight first: api preflight task-leases\n[Main] Preview repair: api repair task-leases --dry-run\n[Main] Execute repair: api repair task-leases --to=pending --yes", reconcileErr)
}
