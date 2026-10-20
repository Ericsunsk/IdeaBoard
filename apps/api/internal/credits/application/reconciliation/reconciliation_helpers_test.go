package reconciliation_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testsupport"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reconciliationEnqueuerStub struct {
	mu    sync.Mutex
	calls []int64
	err   error
}

func (e *reconciliationEnqueuerStub) DispatchReconciliationRun(_ context.Context, _ pgx.Tx, runID int64) error {
	if e.err != nil {
		return e.err
	}
	e.mu.Lock()
	e.calls = append(e.calls, runID)
	e.mu.Unlock()
	return nil
}

func newReconciliationService(
	t testing.TB,
	pool *pgxpool.Pool,
	dispatcher ports.ReconciliationRunDispatcher,
) *credits.Service {
	t.Helper()
	service, err := credits.NewServiceWithReconciliation(pool, dispatcher)
	if err != nil {
		t.Fatalf("credits.NewServiceWithReconciliation() error = %v", err)
	}
	return service
}

func createReconciliationTaskFixture(t *testing.T, db *pgxpool.Pool, service *credits.Service, taskID, status, errorMessage string, userID, priceID, priceVersion int64) string {
	t.Helper()
	hold, err := service.Hold(t.Context(), ports.HoldCommand{UserID: userID, Amount: testhelpers.MustCreditAmount(t, 5), Metadata: taskReconciliationMetadata("hold:"+taskID, taskID)})
	if err != nil {
		t.Fatal(err)
	}
	insertReconciliationWorkspaceTask(t, db, taskID, userID, status, errorMessage)
	insertReconciliationTaskBillingSnapshot(t, db, taskID, userID, priceID, priceVersion, hold.TransactionID)
	if _, err := db.Exec(t.Context(), `INSERT INTO task_billing_settlements(task_id,state) VALUES ($1,'held')`, taskID); err != nil {
		t.Fatal(err)
	}
	return hold.TransactionID
}

// insertReconciliationWorkspaceTask inserts a workspace-scoped tasks row for
// taskID, which is also the row's public_id (task_...) and the key carried by
// task_billing_snapshots.task_id and the Credit Ledger business reference.

func insertReconciliationWorkspaceTask(t testing.TB, db *pgxpool.Pool, taskID string, userID int64, status, errorMessage string) {
	t.Helper()
	workspaceID, billingAccountID := ensureReconciliationWorkspace(t, db, userID)
	completedAt := "NULL"
	if status == "completed" || status == "failed" {
		completedAt = "clock_timestamp()"
	}
	query := `INSERT INTO tasks (public_id,workspace_id,submitter_user_id,billing_account_id,service,workflow_input,status,error_message,completed_at) VALUES ($1,$2,$3,$4,'test-feature','{}',$5,$6,` + completedAt + `)`
	if _, err := db.Exec(t.Context(), query, taskID, workspaceID, userID, billingAccountID, status, errorMessage); err != nil {
		t.Fatal(err)
	}
}

// ensureReconciliationWorkspace returns one personal Workspace and its
// billing account for userID, creating them on first use. Tasks rows require
// both foreign keys; the pair is shared by every fixture task of the test.

func ensureReconciliationWorkspace(t testing.TB, db *pgxpool.Pool, userID int64) (int64, int64) {
	t.Helper()
	ctx := t.Context()
	suffix := strconv.FormatInt(userID, 10)
	if _, err := db.Exec(ctx, `
INSERT INTO workspaces (public_id, owner_user_id, name)
SELECT $1, $2, 'reconciliation fixture'
WHERE NOT EXISTS (
  SELECT 1 FROM workspaces WHERE owner_user_id = $2 AND organization_id IS NULL
)`, "ws_reconciliation_"+suffix, userID); err != nil {
		t.Fatal(err)
	}
	var workspaceID int64
	if err := db.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE owner_user_id = $1 AND organization_id IS NULL`, userID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO billing_accounts (public_id, workspace_id)
SELECT $1, $2
WHERE NOT EXISTS (SELECT 1 FROM billing_accounts WHERE workspace_id = $2)`, "ba_reconciliation_"+suffix, workspaceID); err != nil {
		t.Fatal(err)
	}
	var billingAccountID int64
	if err := db.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE workspace_id = $1`, workspaceID).Scan(&billingAccountID); err != nil {
		t.Fatal(err)
	}
	return workspaceID, billingAccountID
}

func insertReconciliationTaskBillingSnapshot(t testing.TB, db *pgxpool.Pool, taskID string, userID, priceID, priceVersion int64, holdTransactionID string) {
	t.Helper()
	result, err := db.Exec(t.Context(), `
INSERT INTO task_billing_snapshots(task_id,user_id,service,sku,display_name,price_version_id,price_version,unit_credits,quantity,total_credits,quoted_at,hold_transaction_id)
SELECT $1, $2, 'test', prices.sku, skus.display_name, prices.price_version_id, prices.version,
       prices.unit_credits, 1, prices.unit_credits, clock_timestamp(), $5
FROM billing_price_versions prices
JOIN billing_skus skus ON skus.sku = prices.sku
WHERE prices.price_version_id = $3 AND prices.version = $4`, taskID, userID, priceID, priceVersion, holdTransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if rows := result.RowsAffected(); rows != 1 {
		t.Fatalf("insert task Billing snapshot rows=%d, want 1", rows)
	}
}

func executeReconciliationRun(t testing.TB, service *credits.Service) int64 {
	t.Helper()
	run, err := service.StartReconciliation(t.Context(), reconciliationStartInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteReconciliation(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	return run.ID
}

func taskReconciliationMetadata(key, taskID string) domain.CommandMetadata {
	return domain.CommandMetadata{IdempotencyKey: key, Actor: domain.Actor{Type: domain.ActorService, ID: "reconciliation-test"}, Source: "test", Reason: "reconciliation matrix", BusinessReference: domain.BusinessReference{Type: "task", ID: taskID}}
}

func reconciliationStartInput(t testing.TB) ports.StartManualReconciliationInput {
	action := testsupport.AdminAction(t, 1)
	return ports.StartManualReconciliationInput{ActorID: action.ActorID()}
}

func countRows(t testing.TB, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	return count
}
