package commands_test

import (
	"ideaboard/api/internal/credits/testhelpers"

	"ideaboard/api/internal/credits"

	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/application/commands"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestAdminCanGrantCreditsAndReadWalletLedger(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()

	result, err := service.Grant(ctx, ports.GrantCommand{
		UserID: 42,
		Amount: testhelpers.MustCreditAmount(t, 100),
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "admin-grant:test-user-42:welcome",
			Actor:          domain.Actor{Type: domain.ActorAdmin, ID: "7"},
			Source:         "admin_grant",
			BusinessReference: domain.BusinessReference{
				Type: "admin_adjustment",
				ID:   "test-user-42-welcome",
			},
		},
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if result.Replayed {
		t.Fatal("first Grant() was reported as replayed")
	}
	if result.Wallet.Available != 100 || result.Wallet.Reserved != 0 {
		t.Fatalf("Grant() wallet = %+v, want available=100 reserved=0", result.Wallet)
	}

	wallet, err := service.GetWallet(ctx, 42)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.Available != 100 || wallet.Reserved != 0 {
		t.Fatalf("wallet = %+v, want available=100 reserved=0", wallet)
	}

	page, err := service.ListLedger(ctx, domain.ListLedgerInput{UserID: 42, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(page.Transactions) != 1 {
		t.Fatalf("ledger transactions = %d, want 1", len(page.Transactions))
	}
	transaction := page.Transactions[0]
	if transaction.Reason != "" {
		t.Fatalf("Admin ledger reason = %q, want empty", transaction.Reason)
	}
	if transaction.TransactionID != result.TransactionID || transaction.CommandType != domain.CommandGrant {
		t.Fatalf("ledger transaction = %+v", transaction)
	}
	if len(transaction.Postings) != 2 {
		t.Fatalf("postings = %d, want 2", len(transaction.Postings))
	}
	var total int64
	postingAmounts := map[domain.AccountRole]int64{}
	for _, posting := range transaction.Postings {
		total += posting.Amount
		postingAmounts[posting.AccountRole] = posting.Amount
	}
	if total != 0 {
		t.Fatalf("posting total = %d, want 0", total)
	}
	if postingAmounts[domain.AccountSystemIssuance] != -100 || postingAmounts[domain.AccountUserAvailable] != 100 {
		t.Fatalf("posting amounts = %#v", postingAmounts)
	}
}

func TestCreditCommandIdempotencyReturnsOriginalResultAndRejectsHashConflict(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	command := ports.GrantCommand{
		UserID:   42,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("credit-command:idempotent", "idempotent-grant"),
	}

	first, err := service.Grant(ctx, command)
	if err != nil {
		t.Fatalf("first Grant() error = %v", err)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   42,
		Amount:   testhelpers.MustCreditAmount(t, 25),
		Metadata: testhelpers.TestMetadata("credit-command:second", "second-grant"),
	}); err != nil {
		t.Fatalf("second independent Grant() error = %v", err)
	}

	replay, err := service.Grant(ctx, command)
	if err != nil {
		t.Fatalf("replayed Grant() error = %v", err)
	}
	if !replay.Replayed || replay.TransactionID != first.TransactionID {
		t.Fatalf("replay = %+v, first = %+v", replay, first)
	}
	if replay.Wallet.Available != 100 {
		t.Fatalf("replay available = %d, want original result 100", replay.Wallet.Available)
	}

	conflicting := command
	conflicting.Amount = testhelpers.MustCreditAmount(t, 101)
	if _, err := service.Grant(ctx, conflicting); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Grant() error = %v, want domain.ErrIdempotencyConflict", err)
	}
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   88,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: command.Metadata,
	}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("cross-wallet conflicting Grant() error = %v, want domain.ErrIdempotencyConflict", err)
	}
}

func TestCreditCommandsHoldCaptureReleaseAndRefund(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	userID := int64(88)

	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("lifecycle:grant", "lifecycle-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 80),
		Metadata: testhelpers.TestMetadata("lifecycle:hold:capture", "lifecycle-hold-capture"),
	})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	if hold.Wallet.Available != 20 || hold.Wallet.Reserved != 80 {
		t.Fatalf("hold wallet = %+v", hold.Wallet)
	}
	capture, err := service.Capture(ctx, ports.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 80),
		Metadata:          testhelpers.TestMetadata("lifecycle:capture", "lifecycle-capture"),
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if capture.Wallet.Available != 20 || capture.Wallet.Reserved != 0 || capture.Amount != 80 {
		t.Fatalf("capture result = %+v", capture)
	}
	if _, err := service.Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 1),
		Metadata:          testhelpers.TestMetadata("lifecycle:release-settled", "lifecycle-release-settled"),
	}); !errors.Is(err, domain.ErrHoldSettled) {
		t.Fatalf("Release(settled hold) error = %v, want domain.ErrHoldSettled", err)
	}
	refund, err := service.Refund(ctx, ports.RefundCommand{
		UserID:              userID,
		ChargeTransactionID: capture.TransactionID,
		Amount:              testhelpers.MustCreditAmount(t, 30),
		Metadata:            testhelpers.TestMetadata("lifecycle:refund", "lifecycle-refund"),
	})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if refund.Wallet.Available != 50 || refund.Wallet.Reserved != 0 {
		t.Fatalf("refund wallet = %+v", refund.Wallet)
	}
	if _, err := service.Refund(ctx, ports.RefundCommand{
		UserID:              userID,
		ChargeTransactionID: capture.TransactionID,
		Amount:              testhelpers.MustCreditAmount(t, 51),
		Metadata:            testhelpers.TestMetadata("lifecycle:refund-too-large", "lifecycle-refund-too-large"),
	}); !errors.Is(err, domain.ErrRefundExceedsCharge) {
		t.Fatalf("Refund(over charge) error = %v, want domain.ErrRefundExceedsCharge", err)
	}

	releasedHold, err := service.Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 10),
		Metadata: testhelpers.TestMetadata("lifecycle:hold:release", "lifecycle-hold-release"),
	})
	if err != nil {
		t.Fatalf("second Hold() error = %v", err)
	}
	release, err := service.Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: releasedHold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 10),
		Metadata:          testhelpers.TestMetadata("lifecycle:release", "lifecycle-release"),
	})
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if release.Wallet.Available != 50 || release.Wallet.Reserved != 0 {
		t.Fatalf("release wallet = %+v", release.Wallet)
	}
	if _, err := service.Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 51),
		Metadata: testhelpers.TestMetadata("lifecycle:insufficient", "lifecycle-insufficient"),
	}); !errors.Is(err, domain.ErrInsufficientCredits) {
		t.Fatalf("Hold(insufficient) error = %v, want domain.ErrInsufficientCredits", err)
	}

	page, err := service.ListLedger(ctx, domain.ListLedgerInput{UserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(page.Transactions) != 6 {
		t.Fatalf("ledger transactions = %d, want 6", len(page.Transactions))
	}
}

func TestCreditHoldSupportsOneCaptureAndOneReleaseWithStableTotals(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	const userID int64 = 889
	testhelpers.EnsureAdminOperationUser(t, pool, userID)

	if _, err := service.Grant(ctx, ports.GrantCommand{UserID: userID, Amount: testhelpers.MustCreditAmount(t, 4), Metadata: testhelpers.TestMetadata("partial:grant", "partial-grant")}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := service.Hold(ctx, ports.HoldCommand{UserID: userID, Amount: testhelpers.MustCreditAmount(t, 4), Metadata: testhelpers.TestMetadata("partial:hold", "partial-hold")})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	capture, err := service.Capture(ctx, ports.CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Amount: testhelpers.MustCreditAmount(t, 2), Metadata: testhelpers.TestMetadata("partial:capture", "partial-capture")})
	if err != nil {
		t.Fatalf("partial Capture() error = %v", err)
	}
	if capture.Amount != 2 || capture.Wallet.Reserved != 2 {
		t.Fatalf("partial capture = %+v", capture)
	}
	release, err := service.Release(ctx, ports.ReleaseCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Amount: testhelpers.MustCreditAmount(t, 2), Metadata: testhelpers.TestMetadata("partial:release", "partial-release")})
	if err != nil {
		t.Fatalf("partial Release() error = %v", err)
	}
	if release.Wallet.Available != 2 || release.Wallet.Reserved != 0 {
		t.Fatalf("partial release = %+v", release)
	}
	var captured, released int64
	var status string
	if err := pool.QueryRow(ctx, `SELECT captured_amount, released_amount, status FROM credit_holds WHERE hold_transaction_id = $1`, hold.TransactionID).Scan(&captured, &released, &status); err != nil {
		t.Fatalf("read partial hold = %v", err)
	}
	if captured != 2 || released != 2 || status != "captured" {
		t.Fatalf("partial hold facts = captured=%d released=%d status=%s", captured, released, status)
	}
}

func TestConcurrentHoldsCannotOverspendOneWallet(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	userID := int64(90)
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("concurrent-holds:grant", "concurrent-holds-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	const attempts = 20
	var wait sync.WaitGroup
	wait.Add(attempts)
	results := make(chan error, attempts)
	for attempt := 0; attempt < attempts; attempt++ {
		attempt := attempt
		go func() {
			defer wait.Done()
			_, err := service.Hold(ctx, ports.HoldCommand{
				UserID: userID,
				Amount: testhelpers.MustCreditAmount(t, 10),
				Metadata: testhelpers.TestMetadata(
					fmt.Sprintf("concurrent-holds:hold:%d", attempt),
					fmt.Sprintf("concurrent-holds-%d", attempt),
				),
			})
			results <- err
		}()
	}
	wait.Wait()
	close(results)

	var succeeded int
	var insufficient int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrInsufficientCredits):
			insufficient++
		default:
			t.Fatalf("concurrent Hold() unexpected error = %v", err)
		}
	}
	if succeeded != 10 || insufficient != 10 {
		t.Fatalf("holds succeeded=%d insufficient=%d, want 10/10", succeeded, insufficient)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.Available != 0 || wallet.Reserved != 100 {
		t.Fatalf("wallet = %+v, want available=0 reserved=100", wallet)
	}
}

func TestDifferentWalletsDoNotShareASystemAccountLock(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	commands := service.InTx(tx)
	if _, err := commands.Grant(ctx, ports.GrantCommand{
		UserID:   91,
		Amount:   testhelpers.MustCreditAmount(t, 10),
		Metadata: testhelpers.TestMetadata("parallel-wallets:first", "parallel-wallets-first"),
	}); err != nil {
		t.Fatalf("transactional Grant(first wallet) error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := service.Grant(ctx, ports.GrantCommand{
			UserID:   92,
			Amount:   testhelpers.MustCreditAmount(t, 10),
			Metadata: testhelpers.TestMetadata("parallel-wallets:second", "parallel-wallets-second"),
		})
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Grant(second wallet) error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second wallet blocked on the first wallet's open system-account transaction")
	}
}

func TestPostgresEnforcesBalancedImmutableLedgerAndProjectionWrites(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := context.Background()
	userID := int64(93)
	grant, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("ledger-invariants:grant", "ledger-invariants-grant"),
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	unbalancedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(unbalanced) error = %v", err)
	}
	var availableAccountID int64
	if err := unbalancedTx.QueryRow(ctx, `
SELECT id FROM credit_accounts
WHERE wallet_user_id = $1 AND account_role = 'user_available'`, userID).Scan(&availableAccountID); err != nil {
		t.Fatalf("load available account error = %v", err)
	}
	if _, err := unbalancedTx.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count,
  available_balance_after, reserved_balance_after
)
VALUES ('ctx_unbalanced', $1, 'correction', 'ledger-invariants:unbalanced', repeat('0', 64),
        'system', 'test', 'integration_test', 'unbalanced transaction', 'test',
        'unbalanced', 1, 2, 101, 0)`, userID); err != nil {
		t.Fatalf("insert unbalanced transaction error = %v", err)
	}
	if _, err := unbalancedTx.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
VALUES ('ctx_unbalanced', $1, 1)`, availableAccountID); err != nil {
		t.Fatalf("insert unbalanced posting error = %v", err)
	}
	if err := unbalancedTx.Commit(ctx); err == nil {
		t.Fatal("unbalanced credit transaction committed")
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet(after rejected transaction) error = %v", err)
	}
	if wallet.Available != 100 {
		t.Fatalf("available after rejected transaction = %d, want 100", wallet.Available)
	}

	balancedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(balanced) error = %v", err)
	}
	defer func() { _ = balancedTx.Rollback(ctx) }()
	var issuanceAccountID int64
	var sinkAccountID int64
	if err := balancedTx.QueryRow(ctx, `
SELECT id FROM credit_accounts
WHERE wallet_user_id IS NULL AND shard_key = $1 AND account_role = 'system_issuance'`, testhelpers.WalletSystemShard(userID)).Scan(&issuanceAccountID); err != nil {
		t.Fatalf("load issuance account error = %v", err)
	}
	if err := balancedTx.QueryRow(ctx, `
SELECT id FROM credit_accounts
WHERE wallet_user_id IS NULL AND shard_key = $1 AND account_role = 'system_sink'`, testhelpers.WalletSystemShard(userID)).Scan(&sinkAccountID); err != nil {
		t.Fatalf("load sink account error = %v", err)
	}
	if _, err := balancedTx.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count,
  available_balance_after, reserved_balance_after
)
VALUES ('ctx_three_postings', $1, 'correction', 'ledger-invariants:three-postings', repeat('1', 64),
        'system', 'test', 'integration_test', 'three posting correction', 'test',
        'three-postings', 10, 3, 107, 0)`, userID); err != nil {
		t.Fatalf("insert three-posting transaction error = %v", err)
	}
	if _, err := balancedTx.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
VALUES
  ('ctx_three_postings', $1, -10),
  ('ctx_three_postings', $2, 7),
  ('ctx_three_postings', $3, 3)`, issuanceAccountID, availableAccountID, sinkAccountID); err != nil {
		t.Fatalf("insert three postings error = %v", err)
	}
	if err := balancedTx.Commit(ctx); err != nil {
		t.Fatalf("balanced three-posting transaction commit error = %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE credit_accounts SET balance = balance + 1 WHERE id = $1`, availableAccountID); err == nil {
		t.Fatal("direct credit balance overwrite succeeded")
	}
	if _, err := pool.Exec(ctx, `UPDATE credit_transactions SET reason = 'changed' WHERE transaction_id = $1`, grant.TransactionID); err == nil {
		t.Fatal("credit transaction mutation succeeded")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM credit_postings WHERE transaction_id = $1`, grant.TransactionID); err == nil {
		t.Fatal("credit posting deletion succeeded")
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
VALUES ($1, $2, 1)`, grant.TransactionID, sinkAccountID); err == nil {
		t.Fatal("posting was appended after its ledger transaction committed")
	}

	page, err := service.ListLedger(ctx, domain.ListLedgerInput{UserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(page.Transactions) != 2 || len(page.Transactions[0].Postings) != 3 {
		t.Fatalf("ledger after correction = %+v", page.Transactions)
	}
}

func TestNormalizeMetadataRequiresBusinessReasonOnlyForNonAdminActors(t *testing.T) {
	metadata := testhelpers.TestMetadata("metadata:reason", "metadata-reason")
	metadata.Actor.Type = domain.ActorAdmin
	metadata.Reason = ""
	if _, err := commands.NormalizeMetadata(metadata); err != nil {
		t.Fatalf("Admin metadata without reason: %v", err)
	}
	metadata.Actor.Type = domain.ActorService
	if _, err := commands.NormalizeMetadata(metadata); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("service metadata without reason = %v, want invalid command", err)
	}
	metadata.Reason = "  Task hold  "
	normalized, err := commands.NormalizeMetadata(metadata)
	if err != nil || normalized.Reason != "Task hold" {
		t.Fatalf("business reason = %q, %v", normalized.Reason, err)
	}
}
