package control

// Control durable idempotency atomicity (#891), against the isolated
// fixture database with the production wiring pieces (guard scope,
// durable ControlStore, credits service, atomic audit sink).
//
// What is proven here:
//   - commit: the durable claim, a credits Grant executed on the
//     guard-scoped context, and the audit participant row become
//     durable together, all visible INSIDE the still-open guard
//     transaction (dedup fact and business outcome atomically
//     associated).
//   - a second claim with the same scope + key + hash loses and
//     observes the completed fact for replay; with a different hash it
//     observes the mismatch the guard renders as 409.
//   - rollback: the claim, the ledger row, and the audit row are all
//     absent (no orphan facts).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/idempotency"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/control"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func controlIdempotencyClaim(operatorUsr, key, hash string) idempotency.ControlClaim {
	return idempotency.ControlClaim{
		Operator:    operatorUsr,
		Operation:   "credits.grant",
		TargetType:  "user",
		TargetID:    "usr_testwallet",
		Key:         key,
		RequestHash: hash,
	}
}

func countControlIdempotencyFacts(t *testing.T, pool *pgxpool.Pool, key string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM control_idempotency_records WHERE idempotency_key = $1`, key).Scan(&count); err != nil {
		t.Fatalf("count idempotency facts: %v", err)
	}
	return count
}

func TestControlIdempotencyClaimCommitsWithBusinessAndAudit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	_, operatorUsr := seedControlAuditUser(t, pool, "ctl-idem-operator", "admin")
	const walletUserID = 4501
	testhelpers.EnsureAdminOperationUser(t, pool, walletUserID)

	scope := NewControlTxScope(pool)
	store := idempotency.NewControlStore(pool)
	creditService := credits.NewService(pool)
	repo := identitypostgres.NewRepository(pool)
	sink := NewControlAuditAtomicSink(audit.NewStore(pool), runtimehttp.NewUsersRepositoryDirectory(repo))

	const hash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	key := fmt.Sprintf("ctl-idem-%d", time.Now().UnixNano())
	requestID := fmt.Sprintf("req-ctl-idem-%d", time.Now().UnixNano())
	claim := controlIdempotencyClaim(operatorUsr, key, hash)

	txCtx, err := scope.Begin(ctx)
	if err != nil {
		t.Fatalf("scope Begin: %v", err)
	}
	guardTx, ok := control.ControlTxFromContext(txCtx)
	if !ok {
		t.Fatal("guard scope did not stage a carrier transaction")
	}
	claimed, err := store.Claim(txCtx, guardTx, claim)
	claim.AttemptToken = claimed.AttemptToken
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Claim: %v", err)
	}
	if !claimed.Winner {
		scope.Rollback(txCtx)
		t.Fatal("first claim must win")
	}
	joinAccountID, err := creditService.ResolvePersonalAccount(txCtx, walletUserID)
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	result, err := creditService.Administrative().Grant(txCtx, ports.GrantCommand{
		BillingAccountID: joinAccountID,
		Amount:           testhelpers.MustCreditAmount(t, 25),
		Metadata:         testhelpers.TestMetadata("control-idem-grant", "control-idem-grant"),
	})
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Grant() in guard scope error = %v", err)
	}
	if err := store.Complete(txCtx, guardTx, claim, idempotency.ControlResult{
		Status:      200,
		ContentType: "application/json; charset=utf-8",
		Body:        `{"data":{"granted":true}}`,
	}); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Complete: %v", err)
	}
	if _, err := sink.AppendControlAuditTx(txCtx, guardTx, controlJoinRecord(operatorUsr, requestID)); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("AppendControlAuditTx: %v", err)
	}
	// Same-transaction visibility: dedup fact, business fact, and audit
	// row are all readable from INSIDE the still-open guard transaction.
	var insideClaim int
	if err := guardTx.QueryRow(txCtx,
		`SELECT count(*) FROM control_idempotency_records WHERE idempotency_key = $1 AND completed`, key).Scan(&insideClaim); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("read claim inside guard tx: %v", err)
	}
	if insideClaim != 1 {
		scope.Rollback(txCtx)
		t.Fatalf("completed claims visible inside guard tx = %d, want 1", insideClaim)
	}
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
	if err := scope.Commit(txCtx); err != nil {
		t.Fatalf("scope Commit: %v", err)
	}
	if got := countControlIdempotencyFacts(t, pool, key); got != 1 {
		t.Fatalf("idempotency facts after commit = %d, want 1", got)
	}
	if got := countControlJoinLedger(t, pool, result.TransactionID); got != 1 {
		t.Fatalf("ledger rows after commit = %d, want 1", got)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 1 {
		t.Fatalf("audit rows after commit = %d, want 1", got)
	}

	// Same scope + key + hash loses and replays; a different hash
	// observes the mismatch the guard renders as 409.
	txCtx2, err := scope.Begin(ctx)
	if err != nil {
		t.Fatalf("scope Begin replay: %v", err)
	}
	guardTx2, ok := control.ControlTxFromContext(txCtx2)
	if !ok {
		scope.Rollback(txCtx2)
		t.Fatal("guard scope did not stage a carrier transaction")
	}
	replay, err := store.Claim(txCtx2, guardTx2, claim)
	if err != nil {
		scope.Rollback(txCtx2)
		t.Fatalf("replay Claim: %v", err)
	}
	if replay.Winner || replay.Existing == nil || !replay.Existing.Completed {
		scope.Rollback(txCtx2)
		t.Fatalf("replay claim = %+v, want loser with completed fact", replay)
	}
	if replay.Existing.RequestHash != hash || replay.Existing.Result.Body != `{"data":{"granted":true}}` {
		scope.Rollback(txCtx2)
		t.Fatalf("replay fact = %+v, want same hash + stored response", replay.Existing)
	}
	divergent := claim
	divergent.RequestHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	conflict, err := store.Claim(txCtx2, guardTx2, divergent)
	if err != nil {
		scope.Rollback(txCtx2)
		t.Fatalf("conflict Claim: %v", err)
	}
	if conflict.Winner || conflict.Existing == nil || conflict.Existing.RequestHash != hash {
		scope.Rollback(txCtx2)
		t.Fatalf("conflict claim = %+v, want loser observing the first hash", conflict)
	}
	scope.Rollback(txCtx2)
}

func TestControlIdempotencyRollbackRemovesClaimBusinessAndAudit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	_, operatorUsr := seedControlAuditUser(t, pool, "ctl-idem-rollback", "admin")
	const walletUserID = 4502
	testhelpers.EnsureAdminOperationUser(t, pool, walletUserID)

	scope := NewControlTxScope(pool)
	store := idempotency.NewControlStore(pool)
	creditService := credits.NewService(pool)
	repo := identitypostgres.NewRepository(pool)
	sink := NewControlAuditAtomicSink(audit.NewStore(pool), runtimehttp.NewUsersRepositoryDirectory(repo))

	key := fmt.Sprintf("ctl-idem-rollback-%d", time.Now().UnixNano())
	requestID := fmt.Sprintf("req-ctl-idem-rollback-%d", time.Now().UnixNano())
	claim := controlIdempotencyClaim(operatorUsr, key, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	txCtx, err := scope.Begin(ctx)
	if err != nil {
		t.Fatalf("scope Begin: %v", err)
	}
	guardTx, ok := control.ControlTxFromContext(txCtx)
	if !ok {
		t.Fatal("guard scope did not stage a carrier transaction")
	}
	if _, err := store.Claim(txCtx, guardTx, claim); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Claim: %v", err)
	}
	joinAccountID, err := creditService.ResolvePersonalAccount(txCtx, walletUserID)
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	result, err := creditService.Administrative().Grant(txCtx, ports.GrantCommand{
		BillingAccountID: joinAccountID,
		Amount:           testhelpers.MustCreditAmount(t, 25),
		Metadata:         testhelpers.TestMetadata("control-idem-rollback", "control-idem-rollback"),
	})
	if err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("Grant() in guard scope error = %v", err)
	}
	if _, err := sink.AppendControlAuditTx(txCtx, guardTx, controlJoinRecord(operatorUsr, requestID)); err != nil {
		scope.Rollback(txCtx)
		t.Fatalf("AppendControlAuditTx: %v", err)
	}
	// The mutation fails after all three writes (audit join failure at
	// the HTTP layer rolls the guard transaction back): nothing survives.
	scope.Rollback(txCtx)
	if got := countControlIdempotencyFacts(t, pool, key); got != 0 {
		t.Fatalf("idempotency facts after rollback = %d, want 0", got)
	}
	if got := countControlJoinLedger(t, pool, result.TransactionID); got != 0 {
		t.Fatalf("ledger rows after rollback = %d, want 0", got)
	}
	if got := countControlAuditByRequestID(t, pool, requestID); got != 0 {
		t.Fatalf("audit rows after rollback = %d, want 0", got)
	}
}
