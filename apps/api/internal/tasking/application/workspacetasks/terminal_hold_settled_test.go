package workspacetasks

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
)

// TestTerminalCaptureOnSettledHoldMapsToStateConflict pins P1-3 caller
// semantics: when the ledger fact already converged (hold settled), a
// terminal Capture/Release rejection must surface as ErrStateConflict
// (409), never as a retryable 503 that would roll the settlement back
// and re-enter the provider.
func TestTerminalCaptureOnSettledHoldMapsToStateConflict(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	// Settle the hold out-of-band so the terminal Capture hits IB006:
	// the fact already succeeded, the task must converge as a conflict.
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	holdID := stored.BillingSnapshot.HoldTransactionID
	total := stored.BillingSnapshot.TotalCredits
	releaseMeta := domain.CommandMetadata{
		IdempotencyKey:    "p1-3-external-release-" + created.ID,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: "7"},
		Source:            "task_submission",
		Reason:            "external settle",
		BusinessReference: domain.BusinessReference{Type: "task", ID: created.ID},
	}
	amount, err := domain.NewCreditAmount(total)
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	if _, err := env.credits.Settlement().Release(context.Background(), ports.ReleaseCommand{
		BillingAccountID: stored.BillingAccountID, HoldTransactionID: holdID,
		Amount: amount, Metadata: releaseMeta,
	}); err != nil {
		t.Fatalf("external Release: %v", err)
	}

	_, err = env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("Complete on settled hold err = %v, want ErrStateConflict", err)
	}
	if errors.Is(err, ErrSettlementUnavailable) {
		t.Fatalf("Complete err = %v, must not be retryable unavailable", err)
	}
}
