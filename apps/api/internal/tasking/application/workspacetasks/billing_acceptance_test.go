package workspacetasks

import (
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/tasking/infrastructure/adapters"
	"ideaboard/api/internal/tasking/settlement"
	"ideaboard/api/internal/workflow"
)

// Pricing regressions cross the production acceptance seam and its real
// Billing/Credits adapters, including rollback of every durable submit fact.
func TestCreateAIImageBillingMatrix(t *testing.T) {
	for _, feature := range workflow.AIImageFeatureSlugs() {
		for _, scenario := range []struct {
			name        string
			unitCredits int64
			wantErr     error
		}{
			{name: "paid", unitCredits: 25},
			{name: "free"},
			{name: "unpriced", wantErr: billing.ErrPricingUnavailable},
			{name: "disabled", unitCredits: 25, wantErr: billing.ErrPricingUnavailable},
			{name: "archived", unitCredits: 25, wantErr: billing.ErrPricingUnavailable},
			{name: "insufficient", unitCredits: 1001, wantErr: ErrInsufficientCredits},
		} {
			t.Run(feature+"/"+scenario.name, func(t *testing.T) {
				ctx := t.Context()
				price := &scenario.unitCredits
				if scenario.name == "unpriced" {
					price = nil
				}
				env := openAtomicEnvWithPrice(t, feature, price)
				if scenario.name == "disabled" {
					if _, err := env.catalog.SetAvailability(ctx, "ai-image:"+feature, false, env.userID); err != nil {
						t.Fatal(err)
					}
				}
				if scenario.name == "archived" {
					registry, err := billing.NewBillableItemRegistry(nil)
					if err != nil {
						t.Fatal(err)
					}
					env.catalog = billingapplication.NewBillableItemCatalog(registry, billingpostgres.NewStore(env.pool))
				}
				deps := env.serviceDeps
				deps.Quoter = &adapters.WorkspaceTaskQuoter{Catalog: env.catalog}
				deps.Holder = adapters.NewWorkspaceTaskHolder(env.credits)
				service := NewService(deps)
				key := "billing-matrix-" + randomAtomicHex(t, 4)
				created, err := service.Create(ctx, CreateInput{
					WorkspaceID: env.ws, OwnerUserID: env.userID,
					Service: feature, Resolution: "1k", Quantity: 1,
				}, key)
				if scenario.wantErr != nil {
					if !errors.Is(err, scenario.wantErr) {
						t.Fatalf("Create error=%v, want %v", err, scenario.wantErr)
					}
					for _, table := range []string{"tasks", "task_submission_idempotency", "task_billing_snapshots", "task_billing_settlements", "settle_enqueue_probe"} {
						var count int
						if err := env.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
							t.Fatal(err)
						}
						if count != 0 {
							t.Fatalf("rejected Create leaked %d rows in %s", count, table)
						}
					}
					if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
						t.Fatalf("rejected Create wallet=(%d,%d), want (1000,0)", available, reserved)
					}
					if env.ledgerCount(t, `command_type = 'hold'`) != 0 || len(service.StagedEvents()) != 0 {
						t.Fatal("rejected Create leaked a Hold or an event")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				stored, err := env.store.GetTask(ctx, env.ws, created.ID)
				if err != nil {
					t.Fatal(err)
				}
				state := "free"
				holdCount := 0
				if scenario.unitCredits > 0 {
					state = "held"
					holdCount = 1
				}
				snapshot := stored.BillingSnapshot
				if stored.OwnerUserID != env.userID || stored.BillingAccountID != env.accountID || snapshot.SKU != "ai-image:"+feature ||
					snapshot.OutputResolution != "1k" || snapshot.UnitCredits != scenario.unitCredits || snapshot.Quantity != 1 ||
					snapshot.TotalCredits != scenario.unitCredits || snapshot.SettlementState != state ||
					(snapshot.HoldTransactionID != "") != (holdCount == 1) {
					t.Fatalf("accepted Task has wrong billing facts: task=%+v snapshot=%+v", stored, snapshot)
				}
				if available, reserved := env.wallet(t); available != 1000-scenario.unitCredits || reserved != scenario.unitCredits {
					t.Fatalf("wallet=(%d,%d), unit credits=%d", available, reserved, scenario.unitCredits)
				}
				if got := env.ledgerCount(t, `idempotency_key = $1`, settlement.TaskHoldKey(env.ws, key)); got != holdCount {
					t.Fatalf("durable Hold count=%d, want %d", got, holdCount)
				}
				var claimedTaskID string
				if err := env.pool.QueryRow(ctx, `SELECT task_public_id FROM task_submission_idempotency WHERE idempotency_key = $1`, key).Scan(&claimedTaskID); err != nil {
					t.Fatal(err)
				}
				if claimedTaskID != created.ID || env.probeCount(t, created.ID) != 1 {
					t.Fatal("accepted Task lost its durable claim or enqueue")
				}
				if _, err := catalog.LoadRoutingSnapshot(ctx, dbsqlc.New(env.pool), created.ID); err != nil {
					t.Fatalf("accepted Task lost its routing snapshot: %v", err)
				}
			})
		}
	}
}

func TestCreatedTaskBillingFactsAreImmutable(t *testing.T) {
	env := openAtomicEnv(t, 0)
	created := env.createPaidTask(t, 1)
	ctx := t.Context()
	if _, err := billingpostgres.NewStore(env.pool).PublishPrice(ctx, billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: "ai-image:render", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "Renamed",
		},
		UnitCredits: 0, ActorID: env.userID,
	}); err != nil {
		t.Fatal(err)
	}
	var displayName string
	if err := env.pool.QueryRow(ctx, `SELECT display_name FROM task_billing_snapshots WHERE task_id = $1`, created.ID).Scan(&displayName); err != nil {
		t.Fatal(err)
	}
	if displayName != "Render" {
		t.Fatalf("snapshot display name=%q after catalog rename, want Render", displayName)
	}
	if _, err := env.pool.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, billing_account_id, service, sku, display_name, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at
)
SELECT 'invalid-initial-settlement', user_id, billing_account_id, service, sku, display_name, price_version_id, price_version,
       unit_credits, quantity, total_credits, quoted_at
FROM task_billing_snapshots WHERE task_id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO task_billing_settlements (task_id, state) VALUES ('invalid-initial-settlement', 'held')`); err == nil {
		t.Fatal("free snapshot accepted held initial settlement")
	}
	for _, statement := range []string{
		`UPDATE task_billing_snapshots SET display_name = 'tampered' WHERE task_id = $1`,
		`UPDATE task_billing_snapshots SET total_credits = 1 WHERE task_id = $1`,
		`DELETE FROM task_billing_snapshots WHERE task_id = $1`,
		`UPDATE task_billing_settlements SET state = 'released', actual_output_count = 0, released_credits = 1, release_transaction_id = 'fake', settled_at = now() WHERE task_id = $1`,
		`DELETE FROM task_billing_settlements WHERE task_id = $1`,
	} {
		if _, err := env.pool.Exec(ctx, statement, created.ID); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %.80s", statement)
		}
	}
}

// TestFundsGatePreflightBranches restores the deleted fundsgate unit
// coverage as a table over the real CheckFundsGate: restricted denies
// first even when the balance is also short, a short balance fails,
// and boundary funding (available == total) passes. Pure function, no
// DB needed, so it runs with or without API_TEST_DATABASE_DSN.
func TestFundsGatePreflightBranches(t *testing.T) {
	for _, tc := range []struct {
		name             string
		available, total int64
		restricted       bool
		wantErr          error
	}{
		{"restricted denies first", 0, 100, true, ErrWalletRestricted},
		{"short balance", 10, 100, false, ErrInsufficientCredits},
		{"boundary funded", 100, 100, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckFundsGate(tc.available, tc.total, tc.restricted)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckFundsGate(%d,%d,restricted=%v) = %v, want %v", tc.available, tc.total, tc.restricted, err, tc.wantErr)
			}
		})
	}
}
