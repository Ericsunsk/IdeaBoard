package adapters

// Workspace task credits bridge (#821, Phase 4-04 of #809).
//
// This holder adapts credits.Commands to workspacetasks.Holder. Only
// Credits owns ledger writes; this seam performs none directly.
//
// The workspacetasks.Holder Capture/Release signature carries no userID,
// but credits Commands require one for wallet resolution. The adapter
// records hold_transaction_id -> userID at Hold time (process-local,
// deleted on Capture/Release) so settlement converges without an
// interface change. A process restart between Hold and terminal settle
// leaves the hold to the 24h stale-hold reconciler (credits ownership),
// never to a second Hold.
//
// The TxHolder variants (HoldInTx/CaptureInTx/ReleaseInTx) take the userID
// explicitly from the settled task row and run through
// credits.Service.InTx(tx), so terminal settlement never depends on the
// process-local map.

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// WorkspaceTaskHolder adapts credits.Commands to workspacetasks.Holder.
type WorkspaceTaskHolder struct {
	credits *credits.Service

	mu    sync.Mutex
	users map[string]int64
}

var _ workspacetasks.Holder = (*WorkspaceTaskHolder)(nil)
var _ workspacetasks.TxHolder = (*WorkspaceTaskHolder)(nil)

func NewWorkspaceTaskHolder(creditService *credits.Service) *WorkspaceTaskHolder {
	return &WorkspaceTaskHolder{credits: creditService, users: make(map[string]int64)}
}

func taskCommandMetadata(idempotencyKey, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    idempotencyKey,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:            "task_submission",
		Reason:            msgTaskCommandMetadata,
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}

func mapHolderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInsufficientCredits) {
		return fmt.Errorf("%w: %w", workspacetasks.ErrInsufficientCredits, err)
	}
	if errors.Is(err, domain.ErrWalletRestricted) {
		return fmt.Errorf("%w: %w", workspacetasks.ErrWalletRestricted, err)
	}
	return err
}

func (h *WorkspaceTaskHolder) Hold(ctx context.Context, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   creditAmount,
		Metadata: taskCommandMetadata(idempotencyKey, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.mu.Lock()
	h.users[result.TransactionID] = userID
	h.mu.Unlock()
	return result.TransactionID, nil
}

func (h *WorkspaceTaskHolder) holderUserID(holdTransactionID string) (int64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	userID, ok := h.users[holdTransactionID]
	return userID, ok
}

func (h *WorkspaceTaskHolder) forgetHold(holdTransactionID string) {
	h.mu.Lock()
	delete(h.users, holdTransactionID)
	h.mu.Unlock()
}

func (h *WorkspaceTaskHolder) Capture(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		h.forgetHold(holdTransactionID)
		return nil
	}
	userID, ok := h.holderUserID(holdTransactionID)
	if !ok {
		return fmt.Errorf("workspace task holder: unknown hold %s: %w", holdTransactionID, workspacetasks.ErrQuoteUnavailable)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Capture(ctx, ports.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, userID),
	})
	if err != nil {
		return mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return nil
}

func (h *WorkspaceTaskHolder) Release(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		h.forgetHold(holdTransactionID)
		return nil
	}
	userID, ok := h.holderUserID(holdTransactionID)
	if !ok {
		return fmt.Errorf("workspace task holder: unknown hold %s: %w", holdTransactionID, workspacetasks.ErrQuoteUnavailable)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, userID),
	})
	if err != nil {
		return mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return nil
}

// HoldInTx reserves Ideas inside the settlement transaction through the
// transaction-scoped Commands (credits.Service.InTx): the hold row commits
// atomically with the task insert. It satisfies workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   creditAmount,
		Metadata: taskCommandMetadata(idempotencyKey, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.mu.Lock()
	h.users[result.TransactionID] = userID
	h.mu.Unlock()
	return result.TransactionID, nil
}

// CaptureInTx converges a hold to revenue inside the settlement
// transaction. The userID comes from the settled task row, never the
// process-local map. It satisfies workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Capture(ctx, ports.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return result.TransactionID, nil
}

// ReleaseInTx returns reserved Ideas inside the settlement transaction. It
// satisfies workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return result.TransactionID, nil
}
