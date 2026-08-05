package credits

import (
	"errors"
	"testing"

	"ideaboard/api/internal/testdb"
)

func TestPurchaseCreditsUsesCallerTransactionAndCanonicalIdempotency(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	if _, err := service.Grant(t.Context(), GrantCommand{
		UserID: 42, Amount: mustCreditAmount(t, 10),
		Metadata: testMetadata("purchase:seed", "purchase-seed"),
	}); err != nil {
		t.Fatalf("seed Wallet: %v", err)
	}

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := CommandsInSQLTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := commands.Purchase(t.Context(), PurchaseCommand{
		UserID: 42, Amount: mustCreditAmount(t, 500), Metadata: purchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("Purchase() error = %v", err)
	}
	if first.CommandType != CommandPurchaseCredit || first.Wallet.Available != 510 || first.Replayed {
		_ = tx.Rollback()
		t.Fatalf("first Purchase() = %+v", first)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Purchase: %v", err)
	}

	replayTx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	replayCommands, err := CommandsInSQLTx(replayTx)
	if err != nil {
		_ = replayTx.Rollback()
		t.Fatal(err)
	}
	replayed, err := replayCommands.Purchase(t.Context(), PurchaseCommand{
		UserID: 42, Amount: mustCreditAmount(t, 500), Metadata: purchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if err != nil {
		_ = replayTx.Rollback()
		t.Fatalf("replayed Purchase() error = %v", err)
	}
	if !replayed.Replayed || replayed.TransactionID != first.TransactionID {
		_ = replayTx.Rollback()
		t.Fatalf("replayed Purchase() = %+v, first = %+v", replayed, first)
	}
	if err := replayTx.Commit(); err != nil {
		t.Fatalf("commit replay: %v", err)
	}

	conflictTx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conflictTx.Rollback()
	conflictCommands, err := CommandsInSQLTx(conflictTx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conflictCommands.Purchase(t.Context(), PurchaseCommand{
		UserID: 42, Amount: mustCreditAmount(t, 501), Metadata: purchaseMetadata("payment-purchase:order-1", "order-1"),
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting Purchase() error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestPurchaseCreditsRequiresStripePaymentServiceMetadata(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	if _, err := service.Grant(t.Context(), GrantCommand{
		UserID: 42, Amount: mustCreditAmount(t, 1),
		Metadata: testMetadata("purchase:validation-seed", "purchase-validation-seed"),
	}); err != nil {
		t.Fatal(err)
	}

	invalid := purchaseMetadata("payment-purchase:invalid", "order-invalid")
	invalid.Actor = Actor{Type: ActorAdmin, ID: "7"}
	if _, err := service.Purchase(t.Context(), PurchaseCommand{
		UserID: 42, Amount: mustCreditAmount(t, 500), Metadata: invalid,
	}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Purchase() error = %v, want ErrInvalidCommand", err)
	}
}

func TestPurchaseCreditsConvergesAfterWalletBecomesRestrictedAndFrozen(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	if _, err := service.Grant(t.Context(), GrantCommand{
		UserID: 42, Amount: mustCreditAmount(t, 1),
		Metadata: testMetadata("purchase:convergence-seed", "purchase-convergence-seed"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestrictWallet(t.Context(), RestrictWalletCommand{
		UserID: 42, Type: "payment_test", Source: "integration_test",
		RelatedObject: BusinessReference{Type: "payment_order", ID: "order-convergence"},
		Reason:        "verify paid order convergence",
		Actor:         Actor{Type: ActorService, ID: "payments"},
	}); err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}
	if err := freezeWalletForTest(t.Context(), service, pool, 42); err != nil {
		t.Fatalf("freeze Wallet: %v", err)
	}

	result, err := service.Purchase(t.Context(), PurchaseCommand{
		UserID: 42, Amount: mustCreditAmount(t, 500),
		Metadata: purchaseMetadata("payment-purchase:order-convergence", "order-convergence"),
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if result.Wallet.Available != 501 {
		t.Fatalf("Purchase() Wallet = %+v, want available=501", result.Wallet)
	}
}

func TestPurchaseRefundSettlesReservedIdeasToIssuanceIdempotently(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	if _, err := service.Grant(t.Context(), GrantCommand{UserID: 42, Amount: mustCreditAmount(t, 100), Metadata: testMetadata("purchase-refund:seed", "purchase-refund-seed")}); err != nil {
		t.Fatal(err)
	}
	hold, err := service.Hold(t.Context(), HoldCommand{UserID: 42, Amount: mustCreditAmount(t, 40), Metadata: CommandMetadata{
		IdempotencyKey: "purchase-refund:hold", Actor: Actor{Type: ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: "reserve unused Ideas", BusinessReference: BusinessReference{Type: "payment_refund", ID: "refund-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	command := PurchaseRefundCommand{UserID: 42, HoldTransactionID: hold.TransactionID, Metadata: CommandMetadata{
		IdempotencyKey: "purchase-refund:settle", Actor: Actor{Type: ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund succeeded", BusinessReference: BusinessReference{Type: "payment_refund", ID: "refund-1"},
	}}
	result, err := service.PurchaseRefund(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.CommandType != CommandPurchaseRefund || result.Amount != 40 || result.Wallet.Available != 60 || result.Wallet.Reserved != 0 {
		t.Fatalf("PurchaseRefund() = %+v", result)
	}
	replayed, err := service.PurchaseRefund(t.Context(), command)
	if err != nil || !replayed.Replayed || replayed.TransactionID != result.TransactionID {
		t.Fatalf("PurchaseRefund() replay=%+v error=%v", replayed, err)
	}
}

func purchaseMetadata(idempotencyKey, orderID string) CommandMetadata {
	return CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          Actor{Type: ActorService, ID: "payments"},
		Source:         "stripe_purchase",
		Reason:         "Stripe payment credited",
		BusinessReference: BusinessReference{
			Type: "payment_order",
			ID:   orderID,
		},
	}
}
