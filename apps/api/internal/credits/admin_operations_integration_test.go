package credits

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminDeductionOnlyConsumesAvailableCredits(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 401
	ensureAdminOperationUser(t, pool, userID)

	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 100), Metadata: testMetadata("admin-deduction:grant", "admin-deduction-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if _, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 70), Metadata: testMetadata("admin-deduction:hold", "admin-deduction-hold")}); err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	result, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 30), Metadata: testMetadata("admin-deduction:deduct", "admin-deduction-deduct")})
	if err != nil {
		t.Fatalf("Deduct() error = %v", err)
	}
	if result.Wallet.Available != 0 || result.Wallet.Reserved != 70 {
		t.Fatalf("deduction wallet = %+v, want available=0 reserved=70", result.Wallet)
	}
	if _, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("admin-deduction:insufficient", "admin-deduction-insufficient")}); !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("Deduct(insufficient) error = %v, want ErrInsufficientCredits", err)
	}
	metadata := testMetadata("admin-deduction:non-admin", "admin-deduction-non-admin")
	metadata.Actor.Type = ActorSystem
	if _, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: metadata}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Deduct(non-admin) error = %v, want ErrInvalidCommand", err)
	}
}

func TestAdminDeductionIsIdempotentAndConcurrentBounded(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 405
	ensureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 110), Metadata: testMetadata("admin-deduction-idempotent:grant", "admin-deduction-idempotent-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	command := DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("admin-deduction-idempotent:deduct", "admin-deduction-idempotent-deduct")}
	first, err := service.Deduct(ctx, command)
	if err != nil {
		t.Fatalf("first Deduct() error = %v", err)
	}
	replay, err := service.Deduct(ctx, command)
	if err != nil || !replay.Replayed || replay.TransactionID != first.TransactionID {
		t.Fatalf("Deduct() replay=%+v error=%v first=%+v", replay, err, first)
	}
	conflict := command
	conflict.Amount = mustCreditAmount(t, 11)
	if _, err := service.Deduct(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Deduct(conflict) error = %v, want ErrIdempotencyConflict", err)
	}
	conflict = command
	conflict.Metadata.BusinessReference.ID = "admin-deduction-idempotent-other-reference"
	if _, err := service.Deduct(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Deduct(reference conflict) error = %v, want ErrIdempotencyConflict", err)
	}

	const attempts = 8
	var wait sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 25), Metadata: testMetadata(fmt.Sprintf("admin-deduction-concurrent:%d", i), fmt.Sprintf("admin-deduction-concurrent-%d", i))})
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, ErrInsufficientCredits) {
			t.Fatalf("concurrent Deduct() error = %v", err)
		}
	}
	if succeeded != 4 { // First idempotent deduction leaves exactly 100 available.
		t.Fatalf("successful concurrent deductions = %d, want 4", succeeded)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.Available != 0 || wallet.Reserved != 0 {
		t.Fatalf("wallet after concurrent deductions = %+v, want available=0 reserved=0", wallet)
	}
}

func TestAdminRefundIsIdempotentAndDoesNotDoubleRefund(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 406
	ensureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 40), Metadata: testMetadata("admin-refund-idempotent:grant", "admin-refund-idempotent-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 40), Metadata: testMetadata("admin-refund-idempotent:hold", "admin-refund-idempotent-hold")})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("admin-refund-idempotent:capture", "admin-refund-idempotent-capture")})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	command := RefundCommand{UserID: userID, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 15), Metadata: testMetadata("admin-refund-idempotent:refund", "admin-refund-idempotent-refund")}
	first, err := service.Refund(ctx, command)
	if err != nil {
		t.Fatalf("first Refund() error = %v", err)
	}
	replay, err := service.Refund(ctx, command)
	if err != nil || !replay.Replayed || replay.TransactionID != first.TransactionID {
		t.Fatalf("Refund() replay=%+v error=%v first=%+v", replay, err, first)
	}
	conflict := command
	conflict.Amount = mustCreditAmount(t, 16)
	if _, err := service.Refund(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Refund(conflict) error = %v, want ErrIdempotencyConflict", err)
	}
	conflict = command
	conflict.Metadata.BusinessReference.ID = "admin-refund-idempotent-other-reference"
	if _, err := service.Refund(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Refund(reference conflict) error = %v, want ErrIdempotencyConflict", err)
	}
	page, err := service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if err != nil || len(page.Transactions) != 1 || page.Transactions[0].Charge == nil {
		t.Fatalf("refund ledger page=%+v error=%v", page, err)
	}
	if page.Transactions[0].Charge.RefundedAmount != 15 || page.Transactions[0].Charge.RemainingRefundable != 25 {
		t.Fatalf("refund charge after replay=%+v", page.Transactions[0].Charge)
	}
}

func TestAdminCorrectionReversesOriginalTransactionWithConcurrentCap(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 407
	ensureAdminOperationUser(t, pool, userID)
	grant, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 40), Metadata: testMetadata("admin-correction:grant", "admin-correction-grant")})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	first, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: grant.TransactionID, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("admin-correction:first", "admin-correction-first")})
	if err != nil {
		t.Fatalf("Correct() error = %v", err)
	}
	if first.CommandType != CommandCorrection || first.Wallet.Available != 30 {
		t.Fatalf("first correction = %+v", first)
	}
	page, err := service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if err != nil || len(page.Transactions) != 1 || page.Transactions[0].RelatedTransactionID != grant.TransactionID || page.Transactions[0].CommandType != CommandCorrection {
		t.Fatalf("correction ledger page=%+v err=%v", page, err)
	}

	const attempts = 4
	var wait sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: grant.TransactionID, Amount: mustCreditAmount(t, 15), Metadata: testMetadata(fmt.Sprintf("admin-correction:concurrent:%d", i), fmt.Sprintf("admin-correction-concurrent-%d", i))})
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("concurrent Correct() error = %v", err)
		}
	}
	if succeeded != 2 {
		t.Fatalf("successful corrections = %d, want 2", succeeded)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil || wallet.Available != 0 {
		t.Fatalf("wallet after correction=%+v err=%v", wallet, err)
	}
}

func TestAdminCorrectionAllowsRewardRedemptionGrantButRejectsOtherUserGrants(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const rewardUserID int64 = 410
	const otherUserID int64 = 411
	ensureAdminOperationUser(t, pool, rewardUserID, otherUserID)

	rewardMetadata := testMetadata("reward-correction:grant", "reward-correction-grant")
	rewardMetadata.Actor = Actor{Type: ActorUser, ID: "410"}
	rewardMetadata.Source = "reward_redemption"
	rewardMetadata.BusinessReference = BusinessReference{Type: "reward_redemption", ID: "910"}
	rewardGrant, err := service.Grant(ctx, GrantCommand{
		UserID: rewardUserID, Amount: mustCreditAmount(t, 30), Metadata: rewardMetadata,
	})
	if err != nil {
		t.Fatalf("reward Grant() error = %v", err)
	}
	corrected, err := service.Correct(ctx, CorrectionCommand{
		UserID: rewardUserID, OriginalTransactionID: rewardGrant.TransactionID,
		Amount:   mustCreditAmount(t, 30),
		Metadata: testMetadata("reward-correction:correct", "reward-correction-correct"),
	})
	if err != nil {
		t.Fatalf("Correct(reward Grant) error = %v", err)
	}
	if corrected.Wallet.Available != 0 || corrected.CommandType != CommandCorrection {
		t.Fatalf("reward Correction = %+v, want zero available and correction", corrected)
	}

	otherMetadata := testMetadata("user-grant-correction:grant", "user-grant-correction-grant")
	otherMetadata.Actor = Actor{Type: ActorUser, ID: "411"}
	otherMetadata.Source = "manual_user_grant"
	otherGrant, err := service.Grant(ctx, GrantCommand{
		UserID: otherUserID, Amount: mustCreditAmount(t, 10), Metadata: otherMetadata,
	})
	if err != nil {
		t.Fatalf("other user Grant() error = %v", err)
	}
	_, err = service.Correct(ctx, CorrectionCommand{
		UserID: otherUserID, OriginalTransactionID: otherGrant.TransactionID,
		Amount:   mustCreditAmount(t, 10),
		Metadata: testMetadata("user-grant-correction:correct", "user-grant-correction-correct"),
	})
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Correct(other user Grant) error = %v, want ErrInvalidCommand", err)
	}
}

func TestAdminCorrectionSupportsDeductionAndRefundDirectionsAndRejectsInvalidOrigins(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 408
	ensureAdminOperationUser(t, pool, userID, 409)
	grant, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("correction-directions:grant", "correction-directions-grant")})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	deduction, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 4), Metadata: testMetadata("correction-directions:deduction", "correction-directions-deduction")})
	if err != nil {
		t.Fatalf("Deduct() error = %v", err)
	}
	if result, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: deduction.TransactionID, Amount: mustCreditAmount(t, 4), Metadata: testMetadata("correction-directions:deduction-correction", "correction-directions-deduction-correction")}); err != nil || result.Wallet.Available != 10 {
		t.Fatalf("Correct(deduction) result=%+v error=%v", result, err)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: grant.TransactionID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("correction-directions:wrong-wallet", "correction-directions-wrong-wallet")}); err != nil {
		t.Fatalf("Correct(grant) error = %v", err)
	}
	page, _ := service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if len(page.Transactions) != 1 || page.Transactions[0].CommandType != CommandCorrection {
		t.Fatalf("deduction correction ledger=%+v", page)
	}
	correctionID := page.Transactions[0].TransactionID
	if _, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: correctionID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("correction-directions:correction-origin", "correction-directions-correction-origin")}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Correct(correction) error=%v, want ErrInvalidCommand", err)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{UserID: userID + 1, OriginalTransactionID: grant.TransactionID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("correction-directions:wrong-wallet-other", "correction-directions-wrong-wallet")}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Correct(wrong wallet) error=%v, want ErrInvalidCommand", err)
	}
	metadata := testMetadata("correction-directions:system-grant", "correction-directions-system-grant")
	metadata.Actor.Type = ActorSystem
	systemGrant, err := service.Grant(ctx, GrantCommand{UserID: 409, Amount: mustCreditAmount(t, 10), Metadata: metadata})
	if err != nil {
		t.Fatalf("system Grant() error=%v", err)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{UserID: 409, OriginalTransactionID: systemGrant.TransactionID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("correction-directions:system-origin", "correction-directions-system-origin")}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Correct(system grant) error=%v, want ErrInvalidCommand", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: 409, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("correction-directions:hold", "correction-directions-hold")})
	if err != nil {
		t.Fatalf("Hold() error=%v", err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: 409, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("correction-directions:capture", "correction-directions-capture")})
	if err != nil {
		t.Fatalf("Capture() error=%v", err)
	}
	refund, err := service.Refund(ctx, RefundCommand{UserID: 409, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 3), Metadata: testMetadata("correction-directions:refund", "correction-directions-refund")})
	if err != nil {
		t.Fatalf("Refund() error=%v", err)
	}
	if result, err := service.Correct(ctx, CorrectionCommand{UserID: 409, OriginalTransactionID: refund.TransactionID, Amount: mustCreditAmount(t, 3), Metadata: testMetadata("correction-directions:refund-correction", "correction-directions-refund-correction")}); err != nil || result.Wallet.Available != 0 {
		t.Fatalf("Correct(refund) result=%+v error=%v", result, err)
	}
	page, err = service.ListLedger(ctx, ListLedgerInput{UserID: 409, Limit: 1})
	if err != nil || len(page.Transactions) != 1 || page.Transactions[0].Charge == nil || page.Transactions[0].Charge.RefundedAmount != 0 || page.Transactions[0].Charge.RemainingRefundable != 10 {
		t.Fatalf("refund correction charge reference page=%+v error=%v", page, err)
	}
	if _, err := service.Refund(ctx, RefundCommand{UserID: 409, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 3), Metadata: testMetadata("correction-directions:refund-after-correction", "correction-directions-refund-after-correction")}); err != nil {
		t.Fatalf("Refund(after correction) error=%v", err)
	}
}

func TestAdminRefundsAreConcurrentBoundedAndAllowedForFrozenWallet(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 402
	ensureAdminOperationUser(t, pool, userID)

	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 100), Metadata: testMetadata("admin-refund:grant", "admin-refund-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 100), Metadata: testMetadata("admin-refund:hold", "admin-refund-hold")})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("admin-refund:capture", "admin-refund-capture")})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if err := freezeWalletForTest(ctx, service, pool, userID); err != nil {
		t.Fatalf("freeze wallet error = %v", err)
	}
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("admin-refund:frozen-grant", "admin-refund-frozen-grant")}); !errors.Is(err, ErrWalletFrozen) {
		t.Fatalf("Grant(frozen) error = %v, want ErrWalletFrozen", err)
	}
	nonAdminRefund := testMetadata("admin-refund:frozen-system", "admin-refund-frozen-system")
	nonAdminRefund.Actor.Type = ActorSystem
	if _, err := service.Refund(ctx, RefundCommand{UserID: userID, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 1), Metadata: nonAdminRefund}); !errors.Is(err, ErrWalletFrozen) {
		t.Fatalf("Refund(frozen, system actor) error = %v, want ErrWalletFrozen", err)
	}

	const attempts = 8
	var wait sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := service.Refund(ctx, RefundCommand{UserID: userID, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 25), Metadata: testMetadata(fmt.Sprintf("admin-refund:%d", i), fmt.Sprintf("admin-refund-%d", i))})
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, ErrRefundExceedsCharge) {
			t.Fatalf("Refund() error = %v", err)
		}
	}
	if succeeded != 4 {
		t.Fatalf("successful refunds = %d, want 4", succeeded)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.Available != 100 || wallet.Reserved != 0 || wallet.Status != WalletStatusFrozen {
		t.Fatalf("wallet = %+v, want frozen available=100 reserved=0", wallet)
	}
}

func TestAdminLedgerPageExposesCrossPageHoldAndChargeReferences(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 403
	ensureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 50), Metadata: testMetadata("admin-reference:grant", "admin-reference-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 50), Metadata: testMetadata("admin-reference:hold", "admin-reference-hold")})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("admin-reference:capture", "admin-reference-capture")})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	page, err := service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if err != nil || len(page.Transactions) != 1 {
		t.Fatalf("capture page = %+v, error = %v", page, err)
	}
	transaction := page.Transactions[0]
	if transaction.TransactionID != capture.TransactionID || transaction.RelatedTransactionID != hold.TransactionID || transaction.Hold == nil || transaction.Charge == nil {
		t.Fatalf("capture references = %+v", transaction)
	}
	if transaction.Hold.Status != "captured" || transaction.Hold.Amount != 50 || transaction.Hold.TerminalTransactionID != capture.TransactionID || transaction.Charge.RemainingRefundable != 50 {
		t.Fatalf("capture reference details = %+v", transaction)
	}
	refund, err := service.Refund(ctx, RefundCommand{UserID: userID, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 20), Metadata: testMetadata("admin-reference:refund", "admin-reference-refund")})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	page, err = service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if err != nil || len(page.Transactions) != 1 {
		t.Fatalf("refund page = %+v, error = %v", page, err)
	}
	transaction = page.Transactions[0]
	if transaction.TransactionID != refund.TransactionID || transaction.RelatedTransactionID != capture.TransactionID || transaction.Charge == nil {
		t.Fatalf("refund references = %+v", transaction)
	}
	if transaction.Charge.Amount != 50 || transaction.Charge.RefundedAmount != 20 || transaction.Charge.RemainingRefundable != 30 {
		t.Fatalf("refund charge details = %+v", transaction.Charge)
	}
}

func TestAnonymizedTombstoneRetainsAdminLedgerReadAndLinkedRefundOnly(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID int64 = 404
	ensureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 20), Metadata: testMetadata("anonymized:grant", "anonymized-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 20), Metadata: testMetadata("anonymized:hold", "anonymized-hold")})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	capture, err := service.Capture(ctx, CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("anonymized:capture", "anonymized-capture")})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	// This is the canonical completed tombstone state created by Identity: PII
	// cleared, a deleted username/storage UID, status=0, and frozen wallet.
	if _, err := pool.Exec(ctx, `
INSERT INTO user_deletion_requests (user_id, phase, completed_at)
VALUES ($1, 'completed', now())`, userID); err != nil {
		t.Fatalf("record completed tombstone error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE users
SET username = 'deleted-user-anon01', auth_user_id = NULL, storage_uid = 'ANON01', avatar = '', status = 0
WHERE id = $1`, userID); err != nil {
		t.Fatalf("anonymize user error = %v", err)
	}
	if err := freezeWalletForTest(ctx, service, pool, userID); err != nil {
		t.Fatalf("freeze tombstone wallet error = %v", err)
	}

	var username string
	var authUserID *string
	var phase string
	if err := pool.QueryRow(ctx, `
SELECT users.username, users.auth_user_id, request.phase
FROM users JOIN user_deletion_requests request ON request.user_id = users.id
WHERE users.id = $1`, userID).Scan(&username, &authUserID, &phase); err != nil {
		t.Fatalf("read tombstone state error = %v", err)
	}
	if username != "deleted-user-anon01" || authUserID != nil || phase != "completed" {
		t.Fatalf("tombstone state username=%q auth=%v phase=%q", username, authUserID, phase)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil || wallet.Status != WalletStatusFrozen {
		t.Fatalf("GetWallet() wallet=%+v error=%v", wallet, err)
	}
	page, err := service.ListLedger(ctx, ListLedgerInput{UserID: userID, Limit: 1})
	if err != nil || len(page.Transactions) != 1 || page.Transactions[0].Charge == nil || page.Transactions[0].Hold == nil {
		t.Fatalf("anonymized ledger page=%+v error=%v", page, err)
	}
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("anonymized:blocked-grant", "anonymized-blocked-grant")}); !errors.Is(err, ErrWalletFrozen) {
		t.Fatalf("Grant(anonymized) error = %v, want ErrWalletFrozen", err)
	}
	if _, err := service.Deduct(ctx, DeductionCommand{UserID: userID, Amount: mustCreditAmount(t, 1), Metadata: testMetadata("anonymized:blocked-deduction", "anonymized-blocked-deduction")}); !errors.Is(err, ErrWalletFrozen) {
		t.Fatalf("Deduct(anonymized) error = %v, want ErrWalletFrozen", err)
	}
	refund, err := service.Refund(ctx, RefundCommand{UserID: userID, ChargeTransactionID: capture.TransactionID, Amount: mustCreditAmount(t, 5), Metadata: testMetadata("anonymized:refund", "anonymized-refund")})
	if err != nil {
		t.Fatalf("Refund(anonymized) error = %v", err)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{UserID: userID, OriginalTransactionID: refund.TransactionID, Amount: mustCreditAmount(t, 5), Metadata: testMetadata("anonymized:refund-correction", "anonymized-refund-correction")}); err != nil {
		t.Fatalf("Correct(anonymized refund) error = %v", err)
	}
	wallet, err = service.GetWallet(ctx, userID)
	if err != nil || wallet.Status != WalletStatusFrozen {
		t.Fatalf("anonymized correction wallet=%+v error=%v", wallet, err)
	}
}

func freezeWalletForTest(ctx context.Context, service *Service, pool *pgxpool.Pool, userID int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := service.LifecycleInTx(tx).SetWalletFrozen(ctx, userID, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensureAdminOperationUser(t *testing.T, pool *pgxpool.Pool, userIDs ...int64) {
	t.Helper()
	for _, userID := range userIDs {
		storageUID := fmt.Sprintf("A%05d", userID)
		if _, err := pool.Exec(t.Context(), `
INSERT INTO users (id, username, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`, userID, fmt.Sprintf("admin-operation-user-%d", userID), storageUID); err != nil {
			t.Fatalf("seed admin operation user %d: %v", userID, err)
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1) ON CONFLICT DO NOTHING`, storageUID); err != nil {
			t.Fatalf("seed admin operation user UID %d: %v", userID, err)
		}
	}
}
