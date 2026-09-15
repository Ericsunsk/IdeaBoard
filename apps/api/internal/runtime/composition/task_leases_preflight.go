package composition

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
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

func runPreflight() {
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

func runRepairTaskLeases(opts repairTaskLeasesOptions) {
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

// resolveRepairTaskLeasesOptions folds flag values into the effective repair
// policy. Preview is the default; mutation requires an explicit confirmation
// pair, and an explicit --dry-run always wins over it.
func resolveRepairTaskLeasesOptions(dryRun, dryRunExplicit bool, to string, yes bool, limit int) repairTaskLeasesOptions {
	opts := repairTaskLeasesOptions{
		DryRun: dryRun,
		To:     strings.ToLower(strings.TrimSpace(to)),
		Yes:    yes,
		Limit:  limit,
	}
	if !dryRunExplicit && yes && opts.To == "pending" {
		opts.DryRun = false
	}
	return opts
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
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer connectCancel()
	db, err := database.NewPostgres(connectCtx, cfg.Database)
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
