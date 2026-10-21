package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/credits/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetCreditHealthSnapshot(ctx context.Context, pool *pgxpool.Pool) (ports.CreditHealthSnapshot, error) {
	if pool == nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: database pool not configured")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: begin snapshot: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credit health read transaction", "err", rollbackErr)
		}
	}()
	active, err := QueryOptionalReconciliationRun(ctx, tx, `SELECT id, status, trigger_type, requested_by, scheduled_for, finding_count, critical_count, isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at FROM credit_reconciliation_runs WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1`)
	if err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: read active run: %w", err)
	}
	successful, err := QueryOptionalReconciliationRun(ctx, tx, `SELECT id, status, trigger_type, requested_by, scheduled_for, finding_count, critical_count, isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at FROM credit_reconciliation_runs WHERE status = 'completed' ORDER BY completed_at DESC, id DESC LIMIT 1`)
	if err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: read latest successful run: %w", err)
	}
	failed, err := QueryOptionalReconciliationRun(ctx, tx, `SELECT id, status, trigger_type, requested_by, scheduled_for, finding_count, critical_count, isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at FROM credit_reconciliation_runs WHERE status = 'failed' ORDER BY completed_at DESC, id DESC LIMIT 1`)
	if err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: read latest failed run: %w", err)
	}
	var snapshot ports.CreditHealthSnapshot
	var snapshotTime time.Time
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::BIGINT, COUNT(*) FILTER (WHERE severity = 'warning')::BIGINT, COUNT(*) FILTER (WHERE severity = 'error')::BIGINT, COUNT(*) FILTER (WHERE severity = 'critical')::BIGINT, COUNT(*) FILTER (WHERE risk = 'high')::BIGINT, (SELECT COUNT(DISTINCT findings.billing_account_id)::BIGINT FROM credit_reconciliation_findings findings JOIN credit_wallet_restrictions restrictions ON restrictions.billing_account_id = findings.billing_account_id AND restrictions.restriction_type = 'ledger_integrity' AND restrictions.source = 'reconciliation' AND restrictions.related_object_type = 'finding' AND restrictions.related_object_id = findings.id::TEXT AND restrictions.released_at IS NULL), transaction_timestamp() FROM credit_reconciliation_findings WHERE status = 'open'`).Scan(&snapshot.OpenFindings.Total, &snapshot.OpenFindings.Warning, &snapshot.OpenFindings.Error, &snapshot.OpenFindings.Critical, &snapshot.OpenFindings.HighRisk, &snapshot.RestrictedWalletCount, &snapshotTime); err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: read current facts: %w", err)
	}
	snapshot.ActiveRun, snapshot.LatestSuccessfulRun, snapshot.LatestFailedRun, snapshot.SnapshotTime = active, successful, failed, snapshotTime
	if err := tx.Commit(ctx); err != nil {
		return ports.CreditHealthSnapshot{}, fmt.Errorf("credits health: commit snapshot: %w", err)
	}
	return snapshot, nil
}
