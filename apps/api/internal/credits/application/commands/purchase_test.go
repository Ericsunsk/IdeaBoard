package commands_test

import (
	"ideaboard/api/internal/credits/testhelpers"

	"ideaboard/api/internal/credits"

	"errors"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestPurchaseCreditsUsesCallerTransactionAndCanonicalIdempotency(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	if _, err := service.Administrative().Grant(t.Context(), ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 10),
		Metadata: testhelpers.TestMetadata("purchase:seed", "purchase-seed"),
	}); err != nil {
		t.Fatalf("seed Wallet: %v", err)
	}

	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := commands.Purchase(t.Context(), ports.PurchaseCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 500), Metadata: testhelpers.PurchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("Purchase() error = %v", err)
	}
	if first.CommandType != domain.CommandPurchaseCredit || first.Wallet.Available != 510 || first.Replayed {
		_ = tx.Rollback(t.Context())
		t.Fatalf("first Purchase() = %+v", first)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit Purchase: %v", err)
	}

	replayTx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replayCommands, err := credits.CommandsInTx(replayTx)
	if err != nil {
		_ = replayTx.Rollback(t.Context())
		t.Fatal(err)
	}
	replayed, err := replayCommands.Purchase(t.Context(), ports.PurchaseCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 500), Metadata: testhelpers.PurchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if err != nil {
		_ = replayTx.Rollback(t.Context())
		t.Fatalf("replayed Purchase() error = %v", err)
	}
	if !replayed.Replayed || replayed.TransactionID != first.TransactionID {
		_ = replayTx.Rollback(t.Context())
		t.Fatalf("replayed Purchase() = %+v, first = %+v", replayed, first)
	}
	if err := replayTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit replay: %v", err)
	}

	conflictTx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer conflictTx.Rollback(t.Context())
	conflictCommands, err := credits.CommandsInTx(conflictTx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conflictCommands.Purchase(t.Context(), ports.PurchaseCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 501), Metadata: testhelpers.PurchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Purchase() error = %v, want domain.ErrIdempotencyConflict", err)
	}
}

func TestPurchaseCreditsRequiresStripePaymentServiceMetadata(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	if _, err := service.Administrative().Grant(t.Context(), ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 1),
		Metadata: testhelpers.TestMetadata("purchase:validation-seed", "purchase-validation-seed"),
	}); err != nil {
		t.Fatal(err)
	}

	invalid := testhelpers.PurchaseMetadata("payment-purchase:invalid", "order-invalid")
	invalid.Actor = domain.Actor{Type: domain.ActorAdmin, ID: "7"}
	if _, err := service.Payment().Purchase(t.Context(), ports.PurchaseCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 500), Metadata: invalid,
	}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("Purchase() error = %v, want domain.ErrInvalidCommand", err)
	}
}

func TestPurchaseCreditsConvergesAfterWalletBecomesRestrictedAndFrozen(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	if _, err := service.Administrative().Grant(t.Context(), ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 1),
		Metadata: testhelpers.TestMetadata("purchase:convergence-seed", "purchase-convergence-seed"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestrictWallet(t.Context(), ports.RestrictWalletCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Type: "payment_test", Source: "integration_test",
		RelatedObject: domain.BusinessReference{Type: "payment_order", ID: "order-convergence"},
		Reason:        "verify paid order convergence",
		Actor:         domain.Actor{Type: domain.ActorService, ID: "payments"},
	}); err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}
	if err := freezeWalletForTest(t.Context(), service, pool, 42); err != nil {
		t.Fatalf("freeze Wallet: %v", err)
	}

	result, err := service.Payment().Purchase(t.Context(), ports.PurchaseCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 500),
		Metadata: testhelpers.PurchaseMetadata("payment-purchase:order-convergence", "order-convergence"),
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if result.Wallet.Available != 501 {
		t.Fatalf("Purchase() Wallet = %+v, want available=501", result.Wallet)
	}
}

func TestPurchaseRefundSettlesReservedIdeasToIssuanceIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	if _, err := service.Administrative().Grant(t.Context(), ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 100), Metadata: testhelpers.TestMetadata("purchase-refund:seed", "purchase-refund-seed")}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestrictWallet(t.Context(), ports.RestrictWalletCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Type: "payment_dispute", Source: "stripe_dispute",
		RelatedObject: domain.BusinessReference{Type: "payment_dispute", ID: "dispute-1"},
		Reason:        "open payment Dispute", Actor: domain.Actor{Type: domain.ActorService, ID: "payments"},
	}); err != nil {
		t.Fatal(err)
	}
	hold, err := service.Payment().PurchaseRefundHold(t.Context(), ports.PurchaseRefundHoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), Amount: testhelpers.MustCreditAmount(t, 40), Metadata: domain.CommandMetadata{
		IdempotencyKey: "purchase-refund:hold", Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: "reserve unused Ideas", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: "refund-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	command := ports.PurchaseRefundCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 42), HoldTransactionID: hold.TransactionID, Metadata: domain.CommandMetadata{
		IdempotencyKey: "purchase-refund:settle", Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund succeeded", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: "refund-1"},
	}}
	result, err := service.Payment().PurchaseRefund(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.CommandType != domain.CommandPurchaseRefund || result.Amount != 40 || result.Wallet.Available != 60 || result.Wallet.Reserved != 0 {
		t.Fatalf("PurchaseRefund() = %+v", result)
	}
	replayed, err := service.Payment().PurchaseRefund(t.Context(), command)
	if err != nil || !replayed.Replayed || replayed.TransactionID != result.TransactionID {
		t.Fatalf("PurchaseRefund() replay=%+v error=%v", replayed, err)
	}
}
