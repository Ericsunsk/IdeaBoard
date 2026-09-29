// Vision settlement owns the durable money movements behind one vision
// search: idempotent reservation (advisory lock + request-hash replay +
// Catalog quote + Credits Hold + operation row in one transaction), success
// capture, failure release, and the mark for reconciliation when settlement
// is uncertain.
//
// Settlement never calls the vision provider; the executor (vision.go /
// vision_provider.go) owns provider I/O. Every Credits write funnels through
// the typed credits.Commands seam via InTx; this package never touches the
// Ledger or balance projections directly.
package application

import (
	"context"
	"errors"
	"fmt"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// reserve replays an existing operation by (user, idempotency key) or locks,
// quotes, holds, and inserts a new pending row atomically. Unpriced catalog
// states degrade to the deferred free-marker path (no ledger row, no hold)
// with distinct sku_not_found / pricing_unavailable / unpriced_zero reasons.
func (s *VisionBillingService) reserve(ctx context.Context, userID int64, key, requestHash string) (visionReservation, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return visionReservation{}, fmt.Errorf("begin vision billing transaction: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("%d:%s", userID, key)); err != nil {
		return visionReservation{}, fmt.Errorf("lock vision operation: %w", err)
	}
	var existing VisionBillingResult
	var existingHash, status, errorCode, errorMessage string
	err = tx.QueryRow(ctx, `
		SELECT operation_id::text, request_hash, status, query, error_code, error_message
		FROM ai_search_operations
		WHERE user_id = $1 AND idempotency_key = $2
		FOR UPDATE`, userID, key).Scan(&existing.OperationID, &existingHash, &status, &existing.Query, &errorCode, &errorMessage)
	if err == nil {
		if existingHash != requestHash {
			return visionReservation{}, ErrVisionIdempotencyConflict
		}
		existing.Status = status
		existing.Replayed = true
		if status == string(VisionOperationFailed) {
			return visionReservation{result: existing, failure: &VisionOperationFailure{Code: errorCode, Message: errorMessage, HTTPStatus: httpStatusForVisionCode(errorCode)}}, nil
		}
		if status != string(VisionOperationPending) {
			return visionReservation{result: existing}, nil
		}
		return visionReservation{result: existing, execute: false}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return visionReservation{}, fmt.Errorf("load vision operation: %w", err)
	}
	quote, err := s.catalog.QuoteAtResolutionInTx(ctx, tx, visionSearchSKU, billing.OutputResolution1K, 1)
	if err != nil || quote.TotalCredits <= 0 {
		if err != nil {
			if reason, ok := visionBillingDeferredReason(err); ok {
				// Catalog lacks a price: allow the user-facing read with
				// a billing_deferred metric + backfill note, proceeding
				// at amount 0 (free-marker: skip ledger + omit the
				// ai_search_operations row which requires unit_credits > 0).
				// 403 wallet-restricted and transient 503 paths below
				// stay fail-closed. The two sentinels stay distinct via
				// the reason label (not merged).
				operationID := uuid.NewString()
				visionBillingDeferredObserve(visionSearchSKU, reason, operationID)
				return visionReservation{result: VisionBillingResult{OperationID: operationID, Status: string(VisionOperationPending)}, execute: true, deferred: true}, nil
			}
			return visionReservation{}, fmt.Errorf("quote vision search: %w", err)
		}
		// Quoted zero/negative: same free-marker degrade (no ledger, no row).
		operationID := uuid.NewString()
		visionBillingDeferredObserve(visionSearchSKU, visionFailureCodeUnpricedZero, operationID)
		return visionReservation{result: VisionBillingResult{OperationID: operationID, Status: string(VisionOperationPending)}, execute: true, deferred: true}, nil
	}
	amount, err := domain.NewCreditAmount(quote.TotalCredits)
	if err != nil {
		return visionReservation{}, ErrVisionPricingUnavailable
	}
	operationID := uuid.NewString()
	hold, err := s.credits.InTx(tx).Hold(ctx, ports.HoldCommand{
		UserID: userID, Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "ai-search:" + operationID + ":hold",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:            "ai_search_vision",
			Reason:            "vision search hold",
			BusinessReference: domain.BusinessReference{Type: "ai_search_operation", ID: operationID},
		},
	})
	if err != nil {
		return visionReservation{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_search_operations
		(operation_id, user_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id, status)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, 'pending')`, operationID, userID, key, requestHash, visionSearchSKU, quote.PriceVersionID, quote.TotalCredits, hold.TransactionID); err != nil {
		return visionReservation{}, fmt.Errorf("insert vision operation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return visionReservation{}, fmt.Errorf("commit vision billing reservation: %w", err)
	}
	return visionReservation{result: VisionBillingResult{OperationID: operationID, Status: string(VisionOperationPending)}, execute: true}, nil
}

// capture settles a succeeded extraction: Credits Capture plus the durable
// succeeded marker commit atomically.
func (s *VisionBillingService) capture(ctx context.Context, operationID string, userID int64, query string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var holdID string
	var amount int64
	if err := tx.QueryRow(ctx, `SELECT hold_transaction_id, unit_credits FROM ai_search_operations WHERE operation_id = $1::uuid AND user_id = $2 AND status = 'pending' FOR UPDATE`, operationID, userID).Scan(&holdID, &amount); err != nil {
		return err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return err
	}
	if _, err := s.credits.InTx(tx).Capture(ctx, ports.CaptureCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "ai-search:" + operationID + ":capture",
			Actor:          domain.Actor{Type: domain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "ai_search_vision", Reason: "vision search completed",
			BusinessReference: domain.BusinessReference{Type: "ai_search_operation", ID: operationID},
		},
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_search_operations SET status = 'succeeded', query = $2, updated_at = now(), settled_at = now() WHERE operation_id = $1::uuid`, operationID, query); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// release settles a deterministically failed extraction: Credits Release
// plus the durable failed marker commit atomically.
func (s *VisionBillingService) release(ctx context.Context, operationID string, userID int64, code, message string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var holdID string
	var amount int64
	if err := tx.QueryRow(ctx, `SELECT hold_transaction_id, unit_credits FROM ai_search_operations WHERE operation_id = $1::uuid AND user_id = $2 AND status = 'pending' FOR UPDATE`, operationID, userID).Scan(&holdID, &amount); err != nil {
		return err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return err
	}
	if _, err := s.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "ai-search:" + operationID + ":release",
			Actor:          domain.Actor{Type: domain.ActorUser, ID: fmt.Sprintf("%d", userID)},
			Source:         "ai_search_vision", Reason: "vision search failed",
			BusinessReference: domain.BusinessReference{Type: "ai_search_operation", ID: operationID},
		},
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_search_operations SET status = 'failed', error_code = $2, error_message = $3, updated_at = now(), settled_at = now() WHERE operation_id = $1::uuid`, operationID, code, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// markReconciliationPending parks an operation whose settlement outcome is
// uncertain (provider timeout, capture/release failure) for the periodic
// reconciliation pass. It only moves pending rows, never terminal ones.
func (s *VisionBillingService) markReconciliationPending(ctx context.Context, operationID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE ai_search_operations SET status = 'reconciliation_pending', updated_at = now() WHERE operation_id = $1::uuid AND status = 'pending'`, operationID)
	return err
}
