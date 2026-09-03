package taskbilling

import (
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAIImageReadinessReturnsQuoteAndWalletAvailability(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, err := AIImageRequest(workflow.FeatureSlugGeneration, "1k", 1)
	if err != nil {
		t.Fatal(err)
	}
	sku, _ := ResolveSKU(request)
	if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, "Pattern", 25)); err != nil {
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
	amount, _ := domain.NewCreditAmount(20)
	if _, err := creditService.InTx(tx).Grant(t.Context(), ports.GrantCommand{UserID: 1, Amount: amount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "readiness-grant", Actor: domain.Actor{Type: domain.ActorSystem, ID: "test"}, Source: "test", Reason: "readiness", BusinessReference: domain.BusinessReference{Type: "test", ID: "readiness"},
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
			name        string
			unitCredits int64
			publish     bool
			registered  bool
			disabled    bool
			wantErr     error
			wantEnough  bool
		}{
			{name: "paid", unitCredits: 25, publish: true, registered: true, wantEnough: false},
			{name: "free", unitCredits: 0, publish: true, registered: true, wantEnough: true},
			{name: "unpriced", registered: true, wantErr: billing.ErrPricingUnavailable},
			{name: "disabled", unitCredits: 25, publish: true, registered: true, disabled: true, wantErr: billing.ErrPricingUnavailable},
			{name: "archived", unitCredits: 25, publish: true, wantErr: billing.ErrPricingUnavailable},
		} {
			scenario := scenario
			t.Run(feature+"/"+scenario.name, func(t *testing.T) {
				_, pool := testdb.OpenWithPool(t)
				billingService := billing.NewService(pool)
				creditService := credits.NewService(pool)
				request, err := AIImageRequest(feature, "1k", 1)
				if err != nil {
					t.Fatal(err)
				}
				sku, err := ResolveSKU(request)
				if err != nil {
					t.Fatal(err)
				}
				definition := featurePriceInput(sku, feature, scenario.unitCredits).Definition
				definitions := []billing.BillableItemDefinition{}
				if scenario.registered {
					definitions = append(definitions, definition)
				}
				registry, err := billing.NewBillableItemRegistry(definitions)
				if err != nil {
					t.Fatal(err)
				}
				catalog := billing.NewBillableItemCatalog(registry, billingService)
				service := NewService(catalog, creditService)
				if scenario.publish {
					if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, feature, scenario.unitCredits)); err != nil {
						t.Fatal(err)
					}
					if scenario.disabled {
						if _, err := catalog.SetAvailability(t.Context(), string(sku), false, 1); err != nil {
							t.Fatal(err)
						}
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
				amount, err := domain.NewCreditAmount(20)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := creditService.InTx(tx).Grant(t.Context(), ports.GrantCommand{UserID: 1, Amount: amount, Metadata: domain.CommandMetadata{
					IdempotencyKey: "readiness-matrix:" + feature + ":" + scenario.name,
					Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"}, Source: "test", Reason: "readiness matrix",
					BusinessReference: domain.BusinessReference{Type: "test", ID: feature + ":" + scenario.name},
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

func TestHoldAndPersistFailsClosedForUnavailableFeatureStates(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		registered bool
		publish    bool
		disabled   bool
	}{
		{name: "unpriced", registered: true},
		{name: "disabled", registered: true, publish: true, disabled: true},
		{name: "archived", publish: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			billingService := billing.NewService(pool)
			creditService := credits.NewService(pool)
			request, err := AIImageRequest(workflow.FeatureSlugInpaint, "1k", 1)
			if err != nil {
				t.Fatal(err)
			}
			sku, err := ResolveSKU(request)
			if err != nil {
				t.Fatal(err)
			}
			definition := featurePriceInput(sku, "局部重绘", 10).Definition
			definitions := []billing.BillableItemDefinition{}
			if scenario.registered {
				definitions = append(definitions, definition)
			}
			registry, err := billing.NewBillableItemRegistry(definitions)
			if err != nil {
				t.Fatal(err)
			}
			catalog := billing.NewBillableItemCatalog(registry, billingService)
			if scenario.publish {
				if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, "局部重绘", 10)); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.disabled {
				if _, err := catalog.SetAvailability(t.Context(), string(sku), false, 1); err != nil {
					t.Fatal(err)
				}
			}

			tx, err := pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(t.Context())
			service := NewService(catalog, creditService)
			if _, err := service.HoldAndPersist(t.Context(), tx, "unavailable-"+scenario.name, 1, request); !errors.Is(err, ErrPricingUnavailable) {
				t.Fatalf("HoldAndPersist error=%v, want ErrPricingUnavailable", err)
			}
			var count int
			if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM task_billing_snapshots WHERE task_id = $1`, "unavailable-"+scenario.name).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("persisted %d task billing snapshots for unavailable feature", count)
			}
		})
	}
}

func TestSettleTerminalUsesPersistedOutputCount(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		outputCount   int
		wantState     SettlementState
		wantAvailable int64
	}{
		{name: "with-output", outputCount: 1, wantState: SettlementStateCaptured, wantAvailable: 75},
		{name: "without-output", outputCount: 0, wantState: SettlementStateReleased, wantAvailable: 100},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			taskID := "settle-terminal-" + testCase.name
			seedTaskBillingFacts(t, pool, taskID, 1, 100, 25)
			insertTerminalSettlementTask(t, db, taskID, workflow.FeatureSlugGeneration)
			for index := 0; index < testCase.outputCount; index++ {
				if _, err := db.ExecContext(t.Context(), `INSERT INTO task_outputs (task_id, output_kind, role, url, result_index) VALUES ($1, 'remote', 'image', $2, $3)`, taskID, fmt.Sprintf("https://example.com/%d.png", index), index); err != nil {
					t.Fatal(err)
				}
			}

			settleTerminalInSQLTx(t, db, taskID)
			settleTerminalInSQLTx(t, db, taskID)

			assertTaskBillingSettlementFacts(t, db, taskID, testCase.wantState, testCase.wantAvailable)
		})
	}
}

func TestSettleTerminalNoOpsForFreeAndNonBillableTasks(t *testing.T) {
	t.Run("free", func(t *testing.T) {
		db, pool := testdb.OpenWithPool(t)
		taskID := "settle-terminal-free"
		seedTaskBillingFacts(t, pool, taskID, 1, 0, 0)

		settleTerminalInSQLTx(t, db, taskID)

		assertTaskBillingSettlementFacts(t, db, taskID, SettlementStateFree, 0)
		var terminalTransactions int
		if err := db.QueryRowContext(t.Context(), `
SELECT count(*)
FROM credit_transactions
WHERE business_reference_id = $1
  AND command_type IN ('capture', 'release')`, taskID).Scan(&terminalTransactions); err != nil {
			t.Fatal(err)
		}
		if terminalTransactions != 0 {
			t.Fatalf("free task wrote %d terminal Credit transactions", terminalTransactions)
		}
	})

	t.Run("non-billable", func(t *testing.T) {
		db, _ := testdb.OpenWithPool(t)
		insertTerminalSettlementTask(t, db, "settle-terminal-local", "crop")

		settleTerminalInSQLTx(t, db, "settle-terminal-local")
	})
}

func TestSettleTerminalFailsClosedForIncompleteBillingFacts(t *testing.T) {
	t.Run("billed task without snapshot", func(t *testing.T) {
		db, _ := testdb.OpenWithPool(t)
		insertTerminalSettlementTask(t, db, "settle-terminal-no-snapshot", workflow.FeatureSlugGeneration)

		assertSettlementIntegrityError(t, db, "settle-terminal-no-snapshot")
	})

	t.Run("snapshot without settlement", func(t *testing.T) {
		db, pool := testdb.OpenWithPool(t)
		taskID := "settle-terminal-no-settlement"
		seedTaskBillingFacts(t, pool, taskID, 1, 0, 0)
		if _, err := db.ExecContext(t.Context(), `DROP TRIGGER task_billing_settlements_state_machine ON task_billing_settlements`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `DELETE FROM task_billing_settlements WHERE task_id = $1`, taskID); err != nil {
			t.Fatal(err)
		}

		assertSettlementIntegrityError(t, db, taskID)
	})
}

func TestDatabaseRejectsTaskBillingFactMutationAndIllegalSettlement(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, _ := AIImageRequest(workflow.FeatureSlugGeneration, "1k", 1)
	sku, _ := ResolveSKU(request)
	if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, "Pattern", 0)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.HoldAndPersist(t.Context(), tx, "immutable-billing-task", 1, request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DisplayName != "Pattern" {
		t.Fatalf("snapshot display name=%q, want Pattern", snapshot.DisplayName)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, "Pattern Generator", 0)); err != nil {
		t.Fatal(err)
	}
	var persistedDisplayName string
	if err := db.QueryRowContext(t.Context(), `
SELECT display_name FROM task_billing_snapshots WHERE task_id = 'immutable-billing-task'`).Scan(&persistedDisplayName); err != nil {
		t.Fatal(err)
	}
	if persistedDisplayName != "Pattern" {
		t.Fatalf("persisted display name=%q after SKU rename, want Pattern", persistedDisplayName)
	}
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, display_name, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at
)
SELECT 'invalid-initial-settlement', 1, feature_slug, sku, display_name, price_version_id, price_version,
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
		`UPDATE task_billing_snapshots SET display_name = 'tampered' WHERE task_id = 'immutable-billing-task'`,
		`UPDATE task_billing_snapshots SET total_credits = 1 WHERE task_id = 'immutable-billing-task'`,
		`DELETE FROM task_billing_snapshots WHERE task_id = 'immutable-billing-task'`,
		`UPDATE task_billing_settlements SET state = 'released', actual_output_count = 0, released_credits = 1, release_transaction_id = 'fake', settled_at = now() WHERE task_id = 'immutable-billing-task'`,
		`DELETE FROM task_billing_settlements WHERE task_id = 'immutable-billing-task'`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %s", fmt.Sprintf("%.80s", statement))
		}
	}
}

func featurePriceInput(sku billing.SKU, displayName string, unitCredits int64) billing.PublishPriceInput {
	return billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: sku, Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: displayName,
		},
		UnitCredits: unitCredits,
		ActorID:     1,
	}
}

func seedTaskBillingFacts(t *testing.T, pool *pgxpool.Pool, taskID string, userID, grant, unitCredits int64) {
	t.Helper()
	billingService := billing.NewService(pool)
	creditService := credits.NewService(pool)
	service := NewService(billingService, creditService)
	request, err := AIImageRequest(workflow.FeatureSlugGeneration, "1k", 1)
	if err != nil {
		t.Fatal(err)
	}
	sku, err := ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.PublishPrice(t.Context(), featurePriceInput(sku, "Pattern", unitCredits)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	if grant > 0 {
		amount, err := domain.NewCreditAmount(grant)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := creditService.InTx(tx).Grant(t.Context(), ports.GrantCommand{
			UserID: userID,
			Amount: amount,
			Metadata: domain.CommandMetadata{
				IdempotencyKey:    "settle-terminal-grant:" + taskID,
				Actor:             domain.Actor{Type: domain.ActorSystem, ID: "test"},
				Source:            "test",
				Reason:            "terminal settlement test",
				BusinessReference: domain.BusinessReference{Type: "test", ID: taskID},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.HoldAndPersist(t.Context(), tx, taskID, userID, request); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func settleTerminalInSQLTx(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := SettleTerminal(t.Context(), tx, taskID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertSettlementIntegrityError(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := SettleTerminal(t.Context(), tx, taskID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("SettleTerminal error=%v, want ErrIntegrity", err)
	}
}

func insertTerminalSettlementTask(t *testing.T, db *sql.DB, taskID, feature string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `
INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, feature_slug, error_msg, result)
VALUES ($1, 1, '', '', '{}', $2, '', '')`, taskID, feature); err != nil {
		t.Fatal(err)
	}
}

func assertTaskBillingSettlementFacts(t *testing.T, db *sql.DB, taskID string, wantState SettlementState, wantAvailable int64) {
	t.Helper()
	var state SettlementState
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM task_billing_settlements WHERE task_id = $1`, taskID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != wantState {
		t.Fatalf("settlement state=%q, want %q", state, wantState)
	}
	var available int64
	if err := db.QueryRowContext(t.Context(), `SELECT balance FROM credit_accounts WHERE wallet_user_id = 1 AND account_role = 'user_available'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != wantAvailable {
		t.Fatalf("available Credits=%d, want %d", available, wantAvailable)
	}
}
