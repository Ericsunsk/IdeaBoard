package adapters

// Workspace task credits bridge (#821, Phase 4-04 of #809).
//
// This holder adapts credits.Commands to workspacetasks.TxHolder. Only
// Credits owns ledger writes; this seam performs none directly.
//
// Commands receive the stable Billing Account and actual actor.
// Transactional commands join the task settlement transaction through
// credits.Service.SettlementInTx(tx); terminal commands use the persisted Task facts.
// Credits sentinels already match the task boundary, so errors propagate
// unchanged without a second mapping.

import (
	"context"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tasking/settlement"

	"github.com/jackc/pgx/v5"
)

// WorkspaceTaskHolder adapts credits.Commands to taskports.TxHolder.
type WorkspaceTaskHolder struct {
	credits *credits.Service
}

var _ taskports.TxHolder = (*WorkspaceTaskHolder)(nil)

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

// HoldInTx reserves Ideas inside the settlement transaction through the
// transaction-scoped Commands (credits.Service.InTx): the hold row commits
// atomically with the task insert. It satisfies taskports.TxHolder.
func (h *WorkspaceTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", tasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", tasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", tasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", tasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.SettlementInTx(tx).Hold(ctx, ports.HoldCommand{
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
// taskports.TxHolder.
//
// A non-positive amount is a no-op returning ("", nil): the Credits
// ledger rejects non-positive commands, so no ledger row and no
// idempotency key are ever written for the zero leg. Terminal settlement
// records the zero leg with a NULL transaction id (see
// settleFreeTerminal); the tx-id/amount correspondence in the settlement
// trigger treats NULL as the only valid zero-leg marker.
func (h *WorkspaceTaskHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", tasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", tasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", tasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", tasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.SettlementInTx(tx).Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata(settlement.TaskCaptureKey(taskID), taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// ReleaseInTx returns reserved Ideas inside the settlement transaction. It
// satisfies taskports.TxHolder.
//
// A non-positive amount is a no-op returning ("", nil): the Credits
// ledger rejects non-positive commands, so no ledger row and no
// idempotency key are ever written for the zero leg. Terminal settlement
// records the zero leg with a NULL transaction id (see
// settleFreeTerminal); the tx-id/amount correspondence in the settlement
// trigger treats NULL as the only valid zero-leg marker.
func (h *WorkspaceTaskHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", tasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", tasks.ErrQuoteUnavailable)
	}
	if billingAccountID <= 0 {
		return "", fmt.Errorf("workspace task holder: billing account required: %w", tasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", tasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.SettlementInTx(tx).Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  billingAccountID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata(settlement.TaskReleaseKey(taskID), taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}
