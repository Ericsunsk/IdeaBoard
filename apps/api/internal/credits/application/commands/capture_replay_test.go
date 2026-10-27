package commands_test

import (
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"
)

// TestCaptureReplayPinsIdempotentAmountPath proves P1-3: a second Capture
// with the identical idempotency key, amount, and BusinessReference replays
// the original fact (Replayed, same TransactionID, balances unchanged);
// the same key with a different amount or BusinessReference conflicts.
func TestCaptureReplayPinsIdempotentAmountPath(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()
	accountID := testhelpers.MustBillingAccount(t, pool, 91701)

	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 100),
		Metadata:         testhelpers.TestMetadata("cap-replay:grant", "cap-replay-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 40),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "cap-replay:hold",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: "91701"},
			Source:            "task_submission",
			Reason:            "task hold",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_cap_replay"},
		},
	})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}

	meta := domain.CommandMetadata{
		IdempotencyKey:    "task-capture:task_cap_replay",
		Actor:             domain.Actor{Type: domain.ActorUser, ID: "91701"},
		Source:            "task_submission",
		Reason:            "task capture",
		BusinessReference: domain.BusinessReference{Type: "task", ID: "task_cap_replay"},
	}
	first, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 15),
		Metadata:          meta,
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if first.Replayed {
		t.Fatal("first Capture() was reported as replayed")
	}

	replay, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 15),
		Metadata:          meta,
	})
	if err != nil {
		t.Fatalf("Capture(replay) error = %v", err)
	}
	if !replay.Replayed || replay.TransactionID != first.TransactionID {
		t.Fatalf("replay = %+v, want replay of %s", replay, first.TransactionID)
	}
	if replay.Amount != 15 {
		t.Fatalf("replay amount = %d, want 15 (amount-consistent)", replay.Amount)
	}

	// Same key, different amount: conflicts, never a second fact.
	diffAmount := meta
	if _, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 16),
		Metadata:          diffAmount,
	}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("Capture(different amount) error = %v, want ErrIdempotencyConflict", err)
	}

	// Same key, different BusinessReference: conflicts as well.
	diffRef := meta
	diffRef.BusinessReference = domain.BusinessReference{Type: "task", ID: "task_other"}
	if _, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 15),
		Metadata:          diffRef,
	}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("Capture(different business reference) error = %v, want ErrIdempotencyConflict", err)
	}

	// Different key after the hold settled past this leg: IB006. The
	// terminal fact already converged; callers must treat it as a state
	// conflict, never as a retryable 503. Drain the hold first so the
	// second leg settles it fully.
	releaseMeta := domain.CommandMetadata{
		IdempotencyKey:    "task-release:task_cap_replay",
		Actor:             domain.Actor{Type: domain.ActorUser, ID: "91701"},
		Source:            "task_submission",
		Reason:            "task release",
		BusinessReference: domain.BusinessReference{Type: "task", ID: "task_cap_replay"},
	}
	if _, err := service.Settlement().Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 25),
		Metadata:          releaseMeta,
	}); err != nil {
		t.Fatalf("Release(remainder) error = %v", err)
	}
	otherMeta := meta
	otherMeta.IdempotencyKey = "task-capture:task_cap_replay:other-key"
	if _, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 1),
		Metadata:          otherMeta,
	}); !errors.Is(err, domain.ErrHoldSettled) {
		t.Fatalf("Capture(after settled, new key) error = %v, want ErrHoldSettled", err)
	}
}
