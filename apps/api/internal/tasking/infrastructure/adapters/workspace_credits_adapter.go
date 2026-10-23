package adapters

// Workspace task credits bridge (#821, Phase 4-04 of #809).
//
// This holder adapts credits.Commands to workspacetasks.Holder. Only
// Credits owns ledger writes; this seam performs none directly.
//
// Both Holder variants receive the stable Billing Account and actual actor.
// Transactional commands join the task settlement transaction through
// credits.Service.InTx(tx); terminal commands use the persisted Task facts.
// Credits sentinels already match the task boundary, so errors propagate
// unchanged without a second mapping.

import (
	"context"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// WorkspaceTaskHolder adapts credits.Commands to workspacetasks.Holder.
type WorkspaceTaskHolder struct {
	credits *credits.Service
}

var _ workspacetasks.Holder = (*WorkspaceTaskHolder)(nil)
var _ workspacetasks.TxHolder = (*WorkspaceTaskHolder)(nil)

func NewWorkspaceTaskHolder(creditService *credits.Service) *WorkspaceTaskHolder {
	return &WorkspaceTaskHolder{credits: creditService}
}

func taskCommandMetadata(idempotencyKey, taskID string, actorUserID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    idempotencyKey,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(actorUserID)},
		Source:            "task_submission",
		Reason:            errorrender.MsgTaskCommandMetadata,
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}

func (h *WorkspaceTaskHolder) Hold(ctx context.Context, billingAccountID, actorUserID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.Hold(ctx, ports.HoldCommand{
		BillingAccountID: billingAccountID,
		Amount:           creditAmount,
		Metadata:         taskCommandMetadata(idempotencyKey, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *WorkspaceTaskHolder) Capture(ctx context.Context, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, actorUserID),
	})
	return err
}

func (h *WorkspaceTaskHolder) Release(ctx context.Context, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, actorUserID),
	})
	return err
}

// HoldInTx reserves Ideas inside the settlement transaction through the
// transaction-scoped Commands (credits.Service.InTx): the hold row commits
// atomically with the task insert. It satisfies workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Hold(ctx, ports.HoldCommand{
		BillingAccountID: billingAccountID,
		Amount:           creditAmount,
		Metadata:         taskCommandMetadata(idempotencyKey, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// CaptureInTx converges a hold to revenue inside the settlement
// transaction using the persisted Task attribution. It satisfies
// workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// ReleaseInTx returns reserved Ideas inside the settlement transaction. It
// satisfies workspacetasks.TxHolder.
func (h *WorkspaceTaskHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}
