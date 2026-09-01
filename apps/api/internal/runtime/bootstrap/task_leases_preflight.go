package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/taskstore"
)

const (
	defaultPreflightSampleLimit = 5
	defaultRepairLimit          = 10000
)

type taskLeasePreflightRepository interface {
	CountRunningTasksMissingLeaseToken(ctx context.Context) (int64, error)
	ListRunningTasksMissingLeaseToken(ctx context.Context, limit int) ([]taskstore.RunningTaskLease, error)
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
		fatal("usage: api preflight task-leases")
	}
	if len(args) > 1 {
		fatal("usage: api preflight task-leases")
	}

	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)

	taskRepo, err := newTaskLeasePreflightTaskRepo(cfg)
	if err != nil {
		fatal("failed to initialize task lease preflight repository", "err", err)
	}
	if err := runTaskLeasePreflight(context.Background(), taskRepo, defaultPreflightSampleLimit); err != nil {
		fatal("task lease preflight failed", "err", err)
	}
}

func runRepair(args []string) {
	opts, err := parseRepairTaskLeasesArgs(args)
	if err != nil {
		fatal("failed to parse repair task-leases arguments", "err", err)
	}
	if opts.Limit <= 0 {
		fatal("repair limit must be greater than 0")
	}
	if !opts.DryRun && (!opts.Yes || opts.To != "pending") {
		fatal("refusing to mutate; use api repair task-leases --to=pending --yes")
	}

	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)

	taskRepo, err := newTaskLeasePreflightTaskRepo(cfg)
	if err != nil {
		fatal("failed to initialize task lease preflight repository", "err", err)
	}

	ctx := context.Background()
	if opts.DryRun {
		slog.Info("task lease repair dry-run mode")
		report, err := reportTaskLeasePreflight(ctx, taskRepo, defaultPreflightSampleLimit)
		if err != nil {
			fatal("task lease repair dry-run failed", "err", err)
		}
		wouldReset := report.Count
		if int64(opts.Limit) < wouldReset {
			wouldReset = int64(opts.Limit)
		}
		slog.Info("task lease repair dry-run completed", "eligible_missing", report.Count, "would_reset_to_pending", wouldReset, "limit", opts.Limit)
		return
	}

	affected, err := taskRepo.ResetRunningTasksMissingLeaseTokenToPending(ctx, opts.Limit)
	if err != nil {
		fatal("task lease repair failed", "err", err)
	}
	slog.Info("task lease repair completed", "reset_to_pending", affected, "limit", opts.Limit)
}

func parseRepairTaskLeasesArgs(args []string) (repairTaskLeasesOptions, error) {
	opts := repairTaskLeasesOptions{
		DryRun: true,
		Limit:  defaultRepairLimit,
	}
	if len(args) == 0 || args[0] != "task-leases" {
		return opts, errors.New("usage: api repair task-leases [--dry-run] [--to=pending --yes] [--limit=N]")
	}
	fs := flag.NewFlagSet("repair task-leases", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
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
	Sample []taskstore.RunningTaskLease
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
		slog.Info("task lease preflight passed", "missing_lease_token", 0)
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
	slog.Warn("task lease preflight found running tasks missing lease token", "count", count, "sample", len(sample))
	for i, row := range sample {
		slog.Warn("task lease preflight sample", "index", i, "task_id", row.TaskID, "account_id", row.AccountID)
	}
	return report, nil
}

func newTaskLeasePreflightTaskRepo(cfg *config.Config) (taskLeasePreflightRepository, error) {
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	repo := taskstore.NewRepository(db)
	casted, ok := any(repo).(taskLeasePreflightRepository)
	if !ok {
		return nil, errors.New("task repository does not implement task-lease preflight methods")
	}
	return casted, nil
}
