package control

// Control audit atomicity, step 3, end to end: the guard mutation, the
// carrier, a domain write, and the audit row share ONE transaction.
//
// What is proven here, against the isolated fixture database, with the
// production wiring pieces (guard scope, credits service, atomic audit
// sink):
//   - commit: a credits Grant executed on the guard-scoped context plus
//     the audit participant row become durable together, and both are
//     already visible INSIDE the still-open guard transaction.
//   - rollback: the Grant ledger row and the audit row are both absent
//     (the step-2 crash window: the domain commit used to land in its
//     own transaction before the guard committed).
//
// The domain write here is the credits ledger (the first migration
// target); every other step-3 port joins through the same carrier, and
// their per-domain suites prove the same visibility contract.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/platform/audit"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/control"
	"ideaboard/api/internal/testdb"
)

func countControlJoinLedger(t *testing.T, pool *pgxpool.Pool, transactionID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM credit_transactions WHERE transaction_id = $1`, transactionID).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}

func controlJoinRecord(operatorUsr, requestID string) control.ControlAuditRecord {
	return control.ControlAuditRecord{
		OperatorType: "human_session",
		OperatorID:   operatorUsr,
		Capability:   "control:credits",
		Operation:    "credits.grant",
		Route:        "POST /credits/grant",
		RequestID:    requestID,
		Outcome:      control.ControlAuditOK,
		Status:       http.StatusOK,
		OccurredAt:   time.Now().UTC(),
	}
}

func TestControlGuardDomainAuditShareOneTransaction(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	_, operatorUsr := seedControlAuditUser(t, pool, "ctl-join-operator", "admin")
	const walletUserID = 4401
	testhelpers.EnsureAdminOperationUser(t, pool, walletUserID)

	scope := NewControlTxScope(pool)
	creditService := credits.NewService(pool)
	repo := identitypostgres.NewRepository(pool)
	sink := NewControlAuditAtomicSink(audit.NewStore(pool), runtimehttp.NewUsersRepositoryDirectory(repo))

	requestID := fmt.Sprintf("req-control-join-%d", time.Now().UnixNano())
	txCtx, err := scope.Begin(ctx)
	if err != nil {
		t.Fatalf("scope Begin: %v", err)
	}
	// The guarded handler runs inside txCtx: the domain write joins the
	// guard transaction through the carrier (no BeginTx of its own).
	joinAccountID, err := creditService.ResolvePersonalAccount(txCtx, walletUserID)
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	result, err := creditService.Administrative().Grant(txCtx, ports.GrantCommand{
		BillingAccountID: joinAccountID,
		Amount:           testhelpers.MustCreditAmount(t, 50),
		Metadata:         testhelpers.TestMetadata("control-join-grant", "control-join-grant"),
	})
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Grant() in guard scope error = %v", err)
	}
	guardTx, ok := control.ControlTxFromContext(txCtx)
	if !ok {
		scope.Rollback(txCtx)
		t.Fatal("guard scope did not stage a carrier transaction")
	}
	durable, err := sink.AppendControlAuditTx(txCtx, guardTx, controlJoinRecord(operatorUsr, requestID))
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("AppendControlAuditTx in guard scope error = %v", err)
	}
	if !durable {
		scope.Rollback(txCtx)
		t.Fatal("audit sink reported slog-only for a resolvable operator, want durable=true")
	}
	// Same-transaction visibility: the business fact AND its audit row
	// are both readable from INSIDE the still-open guard transaction.
	var insideBiz int
	if err := guardTx.QueryRow(txCtx,
		`SELECT count(*) FROM credit_transactions WHERE transaction_id = $1`, result.TransactionID).Scan(&insideBiz); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("read ledger inside guard tx: %v", err)
	}
	if insideBiz != 1 {
		scope.Rollback(txCtx)
		t.Fatalf("ledger rows visible inside guard tx = %d, want 1", insideBiz)
	}
	var insideAudit int
	if err := guardTx.QueryRow(txCtx,
		`SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&insideAudit); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("read audit inside guard tx: %v", err)
	}
	if insideAudit != 1 {
		scope.Rollback(txCtx)
		t.Fatalf("audit rows visible inside guard tx = %d, want 1", insideAudit)
	}
	if err := scope.Commit(txCtx); err != nil {
		t.Fatalf("scope Commit: %v", err)
	}
	if got := countControlJoinLedger(t, pool, result.TransactionID); got != 1 {
		t.Fatalf("ledger rows after guard commit = %d, want 1", got)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 1 {
		t.Fatalf("audit rows after guard commit = %d, want 1 (business and audit commit together)", got)
	}
}

func TestControlGuardRollbackRemovesDomainAndAudit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	_, operatorUsr := seedControlAuditUser(t, pool, "ctl-join-rollback", "admin")
	const walletUserID = 4402
	testhelpers.EnsureAdminOperationUser(t, pool, walletUserID)

	scope := NewControlTxScope(pool)
	creditService := credits.NewService(pool)
	repo := identitypostgres.NewRepository(pool)
	sink := NewControlAuditAtomicSink(audit.NewStore(pool), runtimehttp.NewUsersRepositoryDirectory(repo))

	requestID := fmt.Sprintf("req-control-join-rollback-%d", time.Now().UnixNano())
	txCtx, err := scope.Begin(ctx)
	if err != nil {
		t.Fatalf("scope Begin: %v", err)
	}
	joinAccountID, err := creditService.ResolvePersonalAccount(txCtx, walletUserID)
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	result, err := creditService.Administrative().Grant(txCtx, ports.GrantCommand{
		BillingAccountID: joinAccountID,
		Amount:           testhelpers.MustCreditAmount(t, 50),
		Metadata:         testhelpers.TestMetadata("control-join-rollback", "control-join-rollback"),
	})
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Grant() in guard scope error = %v", err)
	}
	guardTx, ok := control.ControlTxFromContext(txCtx)
	if !ok {
		scope.Rollback(txCtx)
		t.Fatal("guard scope did not stage a carrier transaction")
	}
	if _, err := sink.AppendControlAuditTx(txCtx, guardTx, controlJoinRecord(operatorUsr, requestID)); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("AppendControlAuditTx in guard scope error = %v", err)
	}
	// The mutation fails after both writes (e.g. the guard's commit
	// fails, or a later participant reports an error): the whole unit,
	// domain write and audit row, rolls back.
	scope.Rollback(txCtx)
	if got := countControlJoinLedger(t, pool, result.TransactionID); got != 0 {
		t.Fatalf("ledger rows after guard rollback = %d, want 0", got)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 0 {
		t.Fatalf("audit rows after guard rollback = %d, want 0 (no orphan audit rows)", got)
	}
}
