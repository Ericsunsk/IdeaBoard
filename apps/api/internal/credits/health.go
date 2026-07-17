package credits

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const creditHealthFreshnessWindow = 48 * time.Hour

type CreditHealthStatus string

const (
	CreditHealthUnchecked CreditHealthStatus = "unchecked"
	CreditHealthHealthy   CreditHealthStatus = "healthy"
	CreditHealthAttention CreditHealthStatus = "attention"
	CreditHealthCritical  CreditHealthStatus = "critical"
	CreditHealthStale     CreditHealthStatus = "stale"
	CreditHealthFailed    CreditHealthStatus = "failed"
)

type CreditHealthFindingSummary struct {
	Total    int64 `json:"total"`
	Warning  int64 `json:"warning"`
	Error    int64 `json:"error"`
	Critical int64 `json:"critical"`
	HighRisk int64 `json:"high_risk"`
}

type CreditHealth struct {
	Status                CreditHealthStatus         `json:"status"`
	ActiveRun             *ReconciliationRun         `json:"active_run,omitempty"`
	LatestSuccessfulRun   *ReconciliationRun         `json:"latest_successful_run,omitempty"`
	LatestFailedRun       *ReconciliationRun         `json:"latest_failed_run,omitempty"`
	OpenFindings          CreditHealthFindingSummary `json:"open_findings"`
	RestrictedWalletCount int64                      `json:"restricted_wallet_count"`
}

type reconciliationRunQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) GetCreditHealth(ctx context.Context) (CreditHealth, error) {
	if s == nil || s.pool == nil {
		return CreditHealth{}, ErrReconciliationUnavailable
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: begin snapshot: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credit health read transaction", "err", rollbackErr)
		}
	}()

	activeRun, err := queryOptionalReconciliationRun(ctx, tx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs
WHERE status IN ('queued', 'running')
ORDER BY id DESC
LIMIT 1`)
	if err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: read active run: %w", err)
	}
	latestSuccessfulRun, err := queryOptionalReconciliationRun(ctx, tx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs
WHERE status = 'completed'
ORDER BY completed_at DESC, id DESC
LIMIT 1`)
	if err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: read latest successful run: %w", err)
	}

	latestFailedRun, err := queryOptionalReconciliationRun(ctx, tx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs
WHERE status = 'failed'
ORDER BY completed_at DESC, id DESC
LIMIT 1`)
	if err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: read latest failed run: %w", err)
	}

	var summary CreditHealthFindingSummary
	var restrictedWalletCount int64
	var snapshotTime time.Time
	if err := tx.QueryRow(ctx, `
SELECT COUNT(*)::BIGINT,
       COUNT(*) FILTER (WHERE severity = 'warning')::BIGINT,
       COUNT(*) FILTER (WHERE severity = 'error')::BIGINT,
       COUNT(*) FILTER (WHERE severity = 'critical')::BIGINT,
       COUNT(*) FILTER (WHERE risk = 'high')::BIGINT,
       (SELECT COUNT(DISTINCT findings.wallet_user_id)::BIGINT
          FROM credit_reconciliation_findings findings
          JOIN credit_wallet_restrictions restrictions
            ON restrictions.wallet_user_id = findings.wallet_user_id
           AND restrictions.restriction_type = 'ledger_integrity'
           AND restrictions.source = 'reconciliation'
           AND restrictions.related_object_type = 'finding'
           AND restrictions.related_object_id = findings.id::TEXT
           AND restrictions.released_at IS NULL),
       transaction_timestamp()
FROM credit_reconciliation_findings
WHERE status = 'open'`).Scan(
		&summary.Total,
		&summary.Warning,
		&summary.Error,
		&summary.Critical,
		&summary.HighRisk,
		&restrictedWalletCount,
		&snapshotTime,
	); err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: read current facts: %w", err)
	}

	health := CreditHealth{
		ActiveRun:             activeRun,
		LatestSuccessfulRun:   latestSuccessfulRun,
		LatestFailedRun:       latestFailedRun,
		OpenFindings:          summary,
		RestrictedWalletCount: restrictedWalletCount,
	}
	health.Status = deriveCreditHealthStatus(snapshotTime, latestSuccessfulRun, latestFailedRun, summary, restrictedWalletCount)
	if err := tx.Commit(ctx); err != nil {
		return CreditHealth{}, fmt.Errorf("credits health: commit snapshot: %w", err)
	}
	return health, nil
}

func queryOptionalReconciliationRun(
	ctx context.Context,
	queryer reconciliationRunQueryer,
	query string,
) (*ReconciliationRun, error) {
	run, err := scanReconciliationRun(queryer.QueryRow(ctx, query))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func deriveCreditHealthStatus(
	now time.Time,
	latestSuccessfulRun *ReconciliationRun,
	latestFailedRun *ReconciliationRun,
	openFindings CreditHealthFindingSummary,
	restrictedWalletCount int64,
) CreditHealthStatus {
	if openFindings.HighRisk > 0 || restrictedWalletCount > 0 {
		return CreditHealthCritical
	}
	if latestFailedRun != nil && latestFailedRun.CompletedAt != nil &&
		(latestSuccessfulRun == nil || latestSuccessfulRun.CompletedAt == nil ||
			latestFailedRun.CompletedAt.After(*latestSuccessfulRun.CompletedAt)) {
		return CreditHealthFailed
	}
	if latestSuccessfulRun == nil || latestSuccessfulRun.CompletedAt == nil {
		return CreditHealthUnchecked
	}
	if now.After(latestSuccessfulRun.CompletedAt.Add(creditHealthFreshnessWindow)) {
		return CreditHealthStale
	}
	if openFindings.Total > 0 {
		return CreditHealthAttention
	}
	return CreditHealthHealthy
}
