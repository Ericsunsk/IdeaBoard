package taskbilling

import (
	"errors"
	"fmt"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
)

func TestAIImageReadinessReturnsQuoteAndWalletAvailability(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, err := AIImageRequest(workflow.FeatureSlugPatternGen, "gpt-image-1")
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

func TestAllAIImageWorkflowReadinessPricingMatrix(t *testing.T) {
	for _, feature := range workflow.AIImageFeatureSlugs() {
		feature := feature
		for _, scenario := range []struct {
			name         string
			unitCredits  int64
			createSKU    bool
			skuEnabled   bool
			priceEnabled bool
			wantErr      error
			wantEnough   bool
		}{
			{name: "paid", unitCredits: 25, createSKU: true, skuEnabled: true, priceEnabled: true, wantEnough: false},
			{name: "free", unitCredits: 0, createSKU: true, skuEnabled: true, priceEnabled: true, wantEnough: true},
			{name: "missing", wantErr: billing.ErrSKUNotFound},
			{name: "disabled-sku", unitCredits: 25, createSKU: true, priceEnabled: true, wantErr: billing.ErrPricingUnavailable},
			{name: "disabled-price", unitCredits: 25, createSKU: true, skuEnabled: true, priceEnabled: false, wantErr: billing.ErrPricingUnavailable},
		} {
			scenario := scenario
			t.Run(feature+"/"+scenario.name, func(t *testing.T) {
				_, pool := testdb.OpenWithPool(t)
				billingService := billing.NewService(pool)
				creditService := credits.NewService(pool)
				service := NewService(billingService, creditService)
				request, err := AIImageRequest(feature, "gpt-image-1")
				if err != nil {
					t.Fatal(err)
				}
				if scenario.createSKU {
					sku, err := ResolveSKU(request)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := billingService.CreateSKU(t.Context(), billing.CreateSKUInput{SKU: string(sku), DisplayName: feature, Enabled: scenario.skuEnabled, ActorID: 1}); err != nil {
						t.Fatal(err)
					}
					if _, err := billingService.CreatePrice(t.Context(), billing.CreatePriceInput{SKU: string(sku), UnitCredits: scenario.unitCredits, Enabled: scenario.priceEnabled, ActorID: 1}); err != nil {
						t.Fatal(err)
					}
				}
				tx, err := pool.Begin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(t.Context())
				if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), 1); err != nil {
					t.Fatal(err)
				}
				amount, err := credits.NewCreditAmount(20)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := creditService.InTx(tx).Grant(t.Context(), credits.GrantCommand{UserID: 1, Amount: amount, Metadata: credits.CommandMetadata{
					IdempotencyKey: "readiness-matrix:" + feature + ":" + scenario.name,
					Actor:          credits.Actor{Type: credits.ActorSystem, ID: "test"}, Source: "test", Reason: "readiness matrix",
					BusinessReference: credits.BusinessReference{Type: "test", ID: feature + ":" + scenario.name},
				}}); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(t.Context()); err != nil {
					t.Fatal(err)
				}

				readiness, err := service.Readiness(t.Context(), 1, request)
				if scenario.wantErr != nil {
					if !errors.Is(err, scenario.wantErr) {
						t.Fatalf("error=%v, want %v", err, scenario.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if readiness.UnitCredits != scenario.unitCredits || readiness.TotalCredits != scenario.unitCredits || readiness.Sufficient != scenario.wantEnough {
					t.Fatalf("readiness=%+v", readiness)
				}
			})
		}
	}
}

func TestDatabaseRejectsTaskBillingFactMutationAndIllegalSettlement(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, _ := AIImageRequest(workflow.FeatureSlugPatternGen, "gpt-image-1")
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
