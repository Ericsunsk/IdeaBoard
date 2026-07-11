package taskbilling

import (
	"fmt"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
)

func TestPatternGenerationReadinessReturnsQuoteAndWalletAvailability(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, err := PatternGenerationRequest("gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}
	sku, _ := ResolveSKU(request)
	if _, err := billingService.CreateSKU(t.Context(), billing.CreateSKUInput{SKU: string(sku), DisplayName: "Pattern", Enabled: true, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.CreatePrice(t.Context(), billing.CreatePriceInput{SKU: string(sku), UnitCredits: 25, Enabled: true, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	amount, _ := credits.NewCreditAmount(20)
	if _, err := creditService.InTx(tx).Grant(t.Context(), credits.GrantCommand{UserID: 1, Amount: amount, Metadata: credits.CommandMetadata{
		IdempotencyKey: "readiness-grant", Actor: credits.Actor{Type: credits.ActorSystem, ID: "test"}, Source: "test", Reason: "readiness", BusinessReference: credits.BusinessReference{Type: "test", ID: "readiness"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	readiness, err := service.Readiness(t.Context(), 1, request)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.SKU != sku || readiness.UnitCredits != 25 || readiness.Quantity != 1 || readiness.TotalCredits != 25 || readiness.AvailableCredits != 20 || readiness.ReservedCredits != 0 || readiness.Sufficient {
		t.Fatalf("readiness=%+v", readiness)
	}
}

func TestDatabaseRejectsTaskBillingFactMutationAndIllegalSettlement(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, _ := PatternGenerationRequest("gpt-image-1")
	sku, _ := ResolveSKU(request)
	if _, err := billingService.CreateSKU(t.Context(), billing.CreateSKUInput{SKU: string(sku), DisplayName: "Pattern", Enabled: true, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.CreatePrice(t.Context(), billing.CreatePriceInput{SKU: string(sku), UnitCredits: 0, Enabled: true, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.HoldAndPersist(t.Context(), tx, "immutable-billing-task", 1, request); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, model, sku, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at
)
SELECT 'invalid-initial-settlement', 1, feature_slug, model, sku, price_version_id, price_version,
       unit_credits, quantity, total_credits, quoted_at
FROM task_billing_snapshots WHERE task_id = 'immutable-billing-task'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO task_billing_settlements (task_id, state)
VALUES ('invalid-initial-settlement', 'held')`); err == nil {
		t.Fatal("free snapshot accepted held initial settlement")
	}
	for _, statement := range []string{
		`UPDATE task_billing_snapshots SET total_credits = 1 WHERE task_id = 'immutable-billing-task'`,
		`DELETE FROM task_billing_snapshots WHERE task_id = 'immutable-billing-task'`,
		`UPDATE task_billing_settlements SET state = 'released', settlement_transaction_id = 'fake', settled_at = now() WHERE task_id = 'immutable-billing-task'`,
		`DELETE FROM task_billing_settlements WHERE task_id = 'immutable-billing-task'`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %s", fmt.Sprintf("%.80s", statement))
		}
	}
}
