package workspacetasks_test

import (
	"context"
	"testing"
)

// Missing mutable billing facts must never be synthesized by terminal settlement.
func TestTerminalSettlementRejectsMissingReadModel(t *testing.T) {
	env := openAtomicEnv(t, 0)
	created := env.createPaidTask(t, 1)
	ctx := context.Background()

	// Copy the committed snapshot under a new task id, leaving its
	// settlement row absent on purpose.
	missingID := "task_missing_settlement"
	if _, err := env.pool.Exec(ctx, `INSERT INTO task_billing_snapshots
(task_id, user_id, billing_account_id, workspace_id, service, sku, display_name, price_version_id, price_version, output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id)
SELECT $1, user_id, billing_account_id, workspace_id, service, sku, display_name, price_version_id, price_version, output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id
FROM task_billing_snapshots WHERE task_id=$2`, missingID, created.ID); err != nil {
		t.Fatalf("seed torn snapshot: %v", err)
	}
	var settlementCount int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_billing_settlements WHERE task_id=$1`, missingID).Scan(&settlementCount); err != nil {
		t.Fatalf("count torn settlement: %v", err)
	}
	if settlementCount != 0 {
		t.Fatalf("torn settlement rows = %d, want 0", settlementCount)
	}

	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := env.store.SetBillingSettlementInTx(ctx, tx, missingID, "released", 0, 0, 0, "", ""); err == nil {
		t.Fatal("missing settlement row must fail closed")
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM task_billing_settlements WHERE task_id=$1`, missingID).Scan(&settlementCount); err != nil {
		t.Fatalf("count settlement after rejected update: %v", err)
	}
	if settlementCount != 0 {
		t.Fatalf("synthesized settlement rows = %d, want 0", settlementCount)
	}
}
