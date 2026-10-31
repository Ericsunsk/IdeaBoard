// Vision settlement owns the durable money movements behind one vision
// search: idempotent reservation (settlement-triple resolution +
// advisory lock + request-hash replay + Catalog quote + Credits Hold +
// operation row in one transaction), success capture, failure release,
// and the mark for reconciliation when settlement is uncertain.
//
// Settlement never calls the vision provider; the executor (vision.go /
// vision_provider.go) owns provider I/O. Every Credits write funnels through
// the typed settlement face (SettlementInTx) via credits.Service; this package never touches the
// Ledger or balance projections directly.
//
// Settlement identity (#918): every operation persists the triple
// (submitter user_id, consumption workspace_id, payer billing_account_id)
// resolved through the stable #915 chain inside the reservation
// transaction. Capture, Release, and background reconciliation converge on
// the STORED payer; they never re-resolve from the live Owner or
// membership, so handovers and departures mid-flight cannot move the
// money. The member cross-workspace quota (#946) joins this same
// transaction after Quote and before Hold.
package vision

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/settlement"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// reserve replays an existing operation by (submitter, idempotency key) or
// locks, resolves the settlement triple, quotes, holds, and inserts a new
// pending row atomically. Unpriced catalog states degrade to the deferred
// free-marker path (no ledger row, no hold) with distinct sku_not_found /
// pricing_unavailable / unpriced_zero reasons.
//
// The replay binds account + resource scope: an existing row whose stored
// workspace, payer, or request hash differs from this call conflicts
// instead of replaying, so a key cannot be replayed across workspaces,
// accounts, operators, or bodies. Insert races (including the legacy
// user-key and the new workspace-key uniques) converge to the same
// conflict.
func (s *VisionBillingService) reserve(ctx context.Context, workspacePublicID string, userID int64, key, requestHash string) (visionReservation, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" || userID <= 0 {
		return visionReservation{}, ErrVisionWorkspaceNotFound
	}
	// Borrow-or-begin on the dbutil interface: the reservation unit joins
	// the ambient guard transaction when one is staged, or opens its own
	// settlement transaction otherwise. With no carrier the owned path
	// below commits and rolls back exactly like the historical
	// pool.BeginTx leg it replaces.
	scope, err := dbutil.Begin(ctx, s.pool, pgx.TxOptions{})
	if err != nil {
		return visionReservation{}, fmt.Errorf("begin vision billing transaction: %w", err)
	}
	tx := scope.Tx()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	// Settlement-triple resolution joins this transaction (membership gate
	// + stable #915 account). Hidden workspaces and non-members fail here
	// with workspace_not_found before any catalog or ledger read, so no
	// payer fact leaks to outsiders. Future 945/946 limit checks join at
	// this same boundary, before Quote/Hold.
	identity, err := s.visionAccounts().ResolveVisionSettlementInTx(ctx, tx, workspacePublicID, userID)
	if err != nil {
		return visionReservation{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("vision:%d:%d:%s", identity.WorkspaceInternalID, identity.BillingAccountID, key)); err != nil {
		return visionReservation{}, fmt.Errorf("lock vision operation: %w", err)
	}
	var existing VisionBillingResult
	var existingHash, status, errorCode, errorMessage string
	var existingWorkspaceID, existingAccountID int64
	err = tx.QueryRow(ctx, `
		SELECT operation_id::text, request_hash, status, query, error_code, error_message, workspace_id, billing_account_id
		FROM ai_search_operations
		WHERE user_id = $1 AND idempotency_key = $2
		FOR UPDATE`, userID, key).Scan(&existing.OperationID, &existingHash, &status, &existing.Query, &errorCode, &errorMessage, &existingWorkspaceID, &existingAccountID)
	if err == nil {
		// Legacy personal rows carry NULL workspace: they never belong to
		// a workspace scope, so they conflict here instead of replaying.
		if existingHash != requestHash || existingWorkspaceID != identity.WorkspaceInternalID || existingAccountID != identity.BillingAccountID {
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
	quote, err := s.catalog.Quote(ctx, tx, visionSearchSKU, billing.OutputResolution1K, 1)
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
	// Member quota gate (#946): after Quote, before Hold, in
	// this same reservation transaction. The ledger Hold that follows is
	// the reservation; terminal Capture/Release needs no re-check and
	// in-flight operations converge on their submit-time facts.
	// A nil checker means unlimited.
	if s.quotas != nil && quote.TotalCredits > 0 {
		if err := s.quotas.CheckQuotaInTx(ctx, tx, identity.BillingAccountID, userID, quote.TotalCredits, quote.QuotedAt); err != nil {
			return visionReservation{}, err
		}
	}
	operationID := uuid.NewString()
	hold, err := s.credits.SettlementInTx(tx).Hold(ctx, ports.HoldCommand{
		BillingAccountID: identity.BillingAccountID, Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    settlement.VisionHoldKey(operationID),
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
		(operation_id, user_id, workspace_id, billing_account_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id, status, created_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11)`, operationID, userID, identity.WorkspaceInternalID, identity.BillingAccountID, key, requestHash, visionSearchSKU, quote.PriceVersionID, quote.TotalCredits, hold.TransactionID, quote.QuotedAt); err != nil {
		if database.IsDuplicateConstraintError(err) {
			return visionReservation{}, ErrVisionIdempotencyConflict
		}
		return visionReservation{}, fmt.Errorf("insert vision operation: %w", err)
	}
	if err := scope.Commit(ctx); err != nil {
		if database.IsDuplicateConstraintError(err) {
			return visionReservation{}, ErrVisionIdempotencyConflict
		}
		return visionReservation{}, fmt.Errorf("commit vision billing reservation: %w", err)
	}
	committed = true
	return visionReservation{result: VisionBillingResult{OperationID: operationID, Status: string(VisionOperationPending)}, execute: true}, nil
}

// capture settles a succeeded extraction: Credits Capture plus the durable
// succeeded marker commit atomically. The payer is the STORED account; the
// caller's workspace scope is verified against the stored row (no
// membership re-gate, no account re-resolution), so post-Hold handovers
// cannot divert the capture.
func (s *VisionBillingService) capture(ctx context.Context, operationID, workspacePublicID string, userID int64, query string) error {
	scope, err := dbutil.Begin(ctx, s.pool, pgx.TxOptions{})
	if err != nil {
		return err
	}
	tx := scope.Tx()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	var holdID string
	var amount int64
	var accountID, workspaceID, submitter int64
	if err := tx.QueryRow(ctx, `SELECT hold_transaction_id, unit_credits, billing_account_id, workspace_id, user_id FROM ai_search_operations WHERE operation_id = $1::uuid AND status = 'pending' FOR UPDATE`, operationID).Scan(&holdID, &amount, &accountID, &workspaceID, &submitter); err != nil {
		return err
	}
	if submitter != userID || !s.verifyVisionWorkspace(ctx, tx, workspacePublicID, workspaceID) {
		return ErrVisionIdempotencyConflict
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return err
	}
	if _, err := s.credits.SettlementInTx(tx).Capture(ctx, ports.CaptureCommand{
		BillingAccountID: accountID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: settlement.VisionCaptureKey(operationID),
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
	if err := scope.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// release settles a deterministically failed extraction: Credits Release
// plus the durable failed marker commit atomically. Same stored-payer
// rule as capture.
func (s *VisionBillingService) release(ctx context.Context, operationID, workspacePublicID string, userID int64, code, message string) error {
	scope, err := dbutil.Begin(ctx, s.pool, pgx.TxOptions{})
	if err != nil {
		return err
	}
	tx := scope.Tx()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	var holdID string
	var amount int64
	var accountID, workspaceID, submitter int64
	if err := tx.QueryRow(ctx, `SELECT hold_transaction_id, unit_credits, billing_account_id, workspace_id, user_id FROM ai_search_operations WHERE operation_id = $1::uuid AND status = 'pending' FOR UPDATE`, operationID).Scan(&holdID, &amount, &accountID, &workspaceID, &submitter); err != nil {
		return err
	}
	if submitter != userID || !s.verifyVisionWorkspace(ctx, tx, workspacePublicID, workspaceID) {
		return ErrVisionIdempotencyConflict
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return err
	}
	if _, err := s.credits.SettlementInTx(tx).Release(ctx, ports.ReleaseCommand{
		BillingAccountID: accountID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: settlement.VisionReleaseKey(operationID),
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
	if err := scope.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// verifyVisionWorkspace binds the settlement to its resource scope without
// re-gating membership: the stored workspace must still resolve to the
// caller's workspace. A workspace deleted mid-flight (hard delete keeps
// the money fact) fails closed into reconciliation, which converges on
// the stored payer without any workspace gate.
func (s *VisionBillingService) verifyVisionWorkspace(ctx context.Context, tx pgx.Tx, workspacePublicID string, storedWorkspaceID int64) bool {
	if storedWorkspaceID <= 0 || strings.TrimSpace(workspacePublicID) == "" {
		return false
	}
	internalID, err := s.visionAccounts().ResolveWorkspaceInternalID(ctx, tx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return false
	}
	return internalID == storedWorkspaceID
}

// visionAccounts returns the injected resolver, defaulting to the
// production #915 chain when unset (constructor always sets it; the
// fallback only guards zero-valued test fixtures).
func (s *VisionBillingService) visionAccounts() VisionBillingAccountResolver {
	if s != nil && s.accounts != nil {
		return s.accounts
	}
	return visionBillingAccountResolver{}
}

// markReconciliationPending parks an operation whose settlement outcome is
// uncertain (provider timeout, capture/release failure) for the periodic
// reconciliation pass. It only moves pending rows, never terminal ones.
func (s *VisionBillingService) markReconciliationPending(ctx context.Context, operationID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE ai_search_operations SET status = 'reconciliation_pending', updated_at = now() WHERE operation_id = $1::uuid AND status = 'pending'`, operationID)
	return err
}
