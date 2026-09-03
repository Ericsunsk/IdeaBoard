package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeoutSeconds = 5

func CreateReconciliationRun(
	ctx context.Context,
	pool *pgxpool.Pool,
	dispatcher ports.ReconciliationRunDispatcher,
	trigger ports.ReconciliationTrigger,
	requestedBy *int64,
	reason *string,
	scheduledFor *time.Time,
	onlyIfOverdue bool,
) (*int64, bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("credits reconciliation: begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeoutSeconds*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits reconciliation transaction", "err", rollbackErr)
		}
	}()
	var runID *int64
	var enqueueRequired bool
	if err := tx.QueryRow(ctx, `SELECT run_id, enqueue_required FROM start_credit_reconciliation($1, $2, $3, $4, $5)`,
		string(trigger), requestedBy, reason, scheduledFor, onlyIfOverdue,
	).Scan(&runID, &enqueueRequired); err != nil {
		return nil, false, fmt.Errorf("credits reconciliation: create run: %w", err)
	}
	if enqueueRequired {
		if runID == nil || *runID <= 0 {
			return nil, false, fmt.Errorf("credits reconciliation: enqueue-required run has invalid ID")
		}
		if dispatcher == nil {
			return nil, false, fmt.Errorf("credits reconciliation: dispatcher not configured")
		}
		if err := dispatcher.DispatchReconciliationRun(ctx, tx, *runID); err != nil {
			return nil, false, fmt.Errorf("credits reconciliation: enqueue run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("credits reconciliation: commit: %w", err)
	}
	return runID, enqueueRequired, nil
}

func ExecuteReconciliation(ctx context.Context, pool *pgxpool.Pool, runID int64) error {
	if pool == nil || runID <= 0 {
		return domain.ErrInvalidCommand
	}
	if _, err := pool.Exec(ctx, `SELECT execute_credit_reconciliation($1)`, runID); err != nil {
		return fmt.Errorf("credits reconciliation: execute: %w", err)
	}
	return nil
}

func FailReconciliation(ctx context.Context, pool *pgxpool.Pool, runID int64, cause error) error {
	if pool == nil || runID <= 0 || cause == nil {
		return domain.ErrInvalidCommand
	}
	_, err := pool.Exec(ctx, `SELECT fail_credit_reconciliation($1, $2)`, runID, cause.Error())
	if err != nil {
		return fmt.Errorf("credits reconciliation: mark run failed: %w", err)
	}
	return nil
}

func GetReconciliationRun(ctx context.Context, pool *pgxpool.Pool, runID int64) (ports.ReconciliationRun, error) {
	if pool == nil || runID <= 0 {
		return ports.ReconciliationRun{}, domain.ErrInvalidCommand
	}
	row := pool.QueryRow(ctx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE id = $1`, runID)
	run, err := ScanReconciliationRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReconciliationRun{}, domain.ErrReconciliationNotFound
	}
	if err != nil {
		return ports.ReconciliationRun{}, fmt.Errorf("credits reconciliation: get run: %w", err)
	}
	return run, nil
}

func ListReconciliationRuns(ctx context.Context, pool *pgxpool.Pool, input ports.ListReconciliationInput) (ports.ReconciliationRunPage, error) {
	limit, err := NormalizeReconciliationLimit(input.Limit)
	if err != nil || input.BeforeID < 0 || pool == nil {
		return ports.ReconciliationRunPage{}, domain.ErrInvalidCommand
	}
	rows, err := pool.Query(ctx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE ($1::BIGINT = 0 OR id < $1) ORDER BY id DESC LIMIT $2`, input.BeforeID, limit+1)
	if err != nil {
		return ports.ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: list runs: %w", err)
	}
	defer rows.Close()
	page := ports.ReconciliationRunPage{Runs: make([]ports.ReconciliationRun, 0, limit)}
	for rows.Next() {
		run, err := ScanReconciliationRun(rows)
		if err != nil {
			return ports.ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: scan run: %w", err)
		}
		page.Runs = append(page.Runs, run)
	}
	if err := rows.Err(); err != nil {
		return ports.ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: read runs: %w", err)
	}
	if len(page.Runs) > limit {
		page.NextBeforeID = page.Runs[limit-1].ID
		page.Runs = page.Runs[:limit]
	}
	return page, nil
}

type reconciliationRunScanner interface {
	Scan(...any) error
}

func ScanReconciliationRun(scanner reconciliationRunScanner) (ports.ReconciliationRun, error) {
	var run ports.ReconciliationRun
	var reason *string
	if err := scanner.Scan(
		&run.ID,
		&run.Status,
		&run.Trigger,
		&run.RequestedBy,
		&reason,
		&run.ScheduledFor,
		&run.FindingCount,
		&run.CriticalCount,
		&run.IsolatedWalletCount,
		&run.AttemptCount,
		&run.ErrorMessage,
		&run.RequestedAt,
		&run.StartedAt,
		&run.CompletedAt,
	); err != nil {
		return ports.ReconciliationRun{}, err
	}
	if reason != nil {
		run.Reason = *reason
	}
	return run, nil
}

func ListGlobalReconciliationFindings(ctx context.Context, pool *pgxpool.Pool, status ports.ReconciliationFindingStatus, input ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error) {
	limit, err := NormalizeReconciliationLimit(input.Limit)
	if err != nil || input.BeforeID < 0 || (status != ports.ReconciliationFindingOpen && status != ports.ReconciliationFindingResolved) || pool == nil {
		return ports.ReconciliationFindingPage{}, domain.ErrInvalidCommand
	}
	rows, err := pool.Query(ctx, `
SELECT id, fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details, status,
       isolated, occurrence_count, first_seen_at, last_seen_at, resolved_at
FROM credit_reconciliation_findings
WHERE status = $1 AND ($2::BIGINT = 0 OR id < $2)
ORDER BY id DESC LIMIT $3`, string(status), input.BeforeID, limit+1)
	if err != nil {
		return ports.ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: list global findings: %w", err)
	}
	defer rows.Close()
	return ScanReconciliationFindingPage(rows, limit)
}

type reconciliationFindingRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func ScanReconciliationFindingPage(rows reconciliationFindingRows, limit int) (ports.ReconciliationFindingPage, error) {
	page := ports.ReconciliationFindingPage{Findings: make([]ports.ReconciliationFinding, 0, limit)}
	for rows.Next() {
		var finding ports.ReconciliationFinding
		if err := rows.Scan(&finding.ID, &finding.Fingerprint, &finding.FindingType, &finding.Severity, &finding.Risk,
			&finding.WalletUserID, &finding.SubjectType, &finding.SubjectID, &finding.Details, &finding.Status,
			&finding.Isolated, &finding.OccurrenceCount, &finding.FirstSeenAt, &finding.LastSeenAt, &finding.ResolvedAt); err != nil {
			return ports.ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: scan finding: %w", err)
		}
		page.Findings = append(page.Findings, finding)
	}
	if err := rows.Err(); err != nil {
		return ports.ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: read findings: %w", err)
	}
	if len(page.Findings) > limit {
		page.NextBeforeID = page.Findings[limit-1].ID
		page.Findings = page.Findings[:limit]
	}
	return page, nil
}

func NormalizeReconciliationLimit(limit int) (int, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit < 1 || limit > 200 {
		return 0, domain.ErrInvalidCommand
	}
	return limit, nil
}

type reconciliationRunQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func QueryOptionalReconciliationRun(
	ctx context.Context,
	queryer reconciliationRunQueryer,
	query string,
) (*ports.ReconciliationRun, error) {
	run, err := ScanReconciliationRun(queryer.QueryRow(ctx, query))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}
