package assets

import (
	"context"
	"fmt"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/derivatives/application/repair"
)

// RequestDerivativeRepair marks a ready derivative whose physical file is
// missing on read as missing, requeues it for regeneration, and enqueues the
// durable outbox event in a single transaction. It is a no-op when another
// worker already moved the row (mark affects zero rows) or the attempt cap is
// reached (the periodic repair worker owns the terminal transition).
func (r *MediaAssetRegistry) RequestDerivativeRepair(ctx context.Context, ownerUserID int64, derivative dbsqlc.MediaDerivative) error {
	if r == nil || r.db == nil || derivative.ID <= 0 || ownerUserID <= 0 {
		return fmt.Errorf("media derivative repair request is invalid")
	}
	if derivative.OwnerUserID != ownerUserID {
		return fmt.Errorf("media derivative repair request is invalid")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media derivative self-heal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := dbsqlc.New(tx)
	marked, err := q.MarkMediaDerivativeMissing(ctx, dbsqlc.MarkMediaDerivativeMissingParams{
		ErrorMessage:     "derivative file missing on read; self-heal requested",
		ID:               derivative.ID,
		ObjectGeneration: derivative.ObjectGeneration,
	})
	if err != nil {
		return fmt.Errorf("mark media derivative missing %d: %w", derivative.ID, err)
	}
	if marked == 0 {
		return nil
	}
	requeued, err := q.RequeueMediaDerivativeRepair(ctx, dbsqlc.RequeueMediaDerivativeRepairParams{
		ID:              derivative.ID,
		MaxAttemptCount: repair.DefaultRepairMaxAttempts,
	})
	if err != nil {
		return fmt.Errorf("requeue media derivative repair %d: %w", derivative.ID, err)
	}
	if requeued == 0 {
		return nil
	}
	if _, err := q.EnqueueMediaDerivativeOutbox(ctx, derivative.ID); err != nil {
		return fmt.Errorf("enqueue media derivative repair %d: %w", derivative.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media derivative self-heal %d: %w", derivative.ID, err)
	}
	return nil
}
