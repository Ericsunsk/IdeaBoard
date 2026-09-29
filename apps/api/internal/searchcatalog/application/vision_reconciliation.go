// Vision reconciliation converges operations left unsettled by uncertain
// provider or settlement outcomes: the periodic worker releases the hold and
// marks the operation failed with a retryable timeout marker. It never
// re-executes the provider call and never captures; late provider results
// cannot resurrect a reconciled row because reconcileOne only moves
// pending/reconciliation_pending rows under row lock.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/jackc/pgx/v5"
)

// ReconcilePending converges every operation older than visionPendingAfter
// still awaiting settlement, at most 100 per pass ordered by age.
func (s *VisionBillingService) ReconcilePending(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT operation_id::text FROM ai_search_operations WHERE status IN ('pending', 'reconciliation_pending') AND created_at < now() - $1::interval ORDER BY created_at LIMIT 100`, fmt.Sprintf("%d minutes", int(visionPendingAfter/time.Minute)))
	if err != nil {
		return err
	}
	defer rows.Close()
	var operationIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		operationIDs = append(operationIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	// Release the read connection before opening one transaction per operation.
	// This keeps reconciliation bounded by the pool even when the batch is full.
	rows.Close()
	for _, id := range operationIDs {
		if err := s.reconcileOne(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// reconcileOne releases one unsettled operation's hold and marks it failed.
// Missing rows (already converged) are acknowledged without error.
func (s *VisionBillingService) reconcileOne(ctx context.Context, operationID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var userID int64
	var holdID string
	var amount int64
	if err := tx.QueryRow(ctx, `SELECT user_id, hold_transaction_id, unit_credits FROM ai_search_operations WHERE operation_id = $1::uuid AND status IN ('pending', 'reconciliation_pending') FOR UPDATE`, operationID).Scan(&userID, &holdID, &amount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return err
	}
	if _, err := s.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "ai-search:" + operationID + ":reconcile-release",
			Actor:          domain.Actor{Type: domain.ActorSystem, ID: "vision-reconciliation"},
			Source:         "ai_search_vision", Reason: "vision search reconciliation timeout",
			BusinessReference: domain.BusinessReference{Type: "ai_search_operation", ID: operationID},
		},
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_search_operations SET status = 'failed', error_code = 'vision_search_reconciliation_timeout', error_message = '视觉搜索超时，请重试', updated_at = now(), settled_at = now() WHERE operation_id = $1::uuid`, operationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
