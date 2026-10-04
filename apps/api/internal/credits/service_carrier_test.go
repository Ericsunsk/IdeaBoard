package credits

// Control audit atomicity, step 3: the credits ports join the ambient
// guard transaction instead of committing in their own.
//
// What is proven here, against the isolated fixture database:
//   - Grant on a carrier context lands inside the carrier transaction:
//     the ledger row is readable from the still-open carrier and
//     invisible on the pool, then durable after the carrier commits.
//   - A carrier rollback leaves no ledger row and no wallet behind
//     (the step-2 gap: the domain commit used to survive on its own).
//   - RestrictWallet / ReleaseWalletRestriction join the same way.
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing service suite is that proof; it is not duplicated here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// carrierQuerier is satisfied by both pgx.Tx (same-transaction reads)
// and *pgxpool.Pool (outside reads) for the visibility assertions below.
type carrierQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func countLedgerByTransactionID(t *testing.T, ctx context.Context, querier carrierQuerier, transactionID string) int {
	t.Helper()
	var count int
	if err := querier.QueryRow(ctx,
		`SELECT count(*) FROM credit_transactions WHERE transaction_id = $1`, transactionID).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}

// beginCarrier stages a guard-style transaction: the test (like the
// guard) owns commit and rollback, while the service under test only
// borrows the transaction through the carrier context.
func beginCarrier(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, context.Context) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	return tx, dbutil.ContextWithAmbientTx(ctx, tx)
}

func TestGrantJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	service := NewService(pool)
	ctx := context.Background()
	testhelpers.EnsureAdminOperationUser(t, pool, 4201)

	tx, carrier := beginCarrier(t, ctx, pool)
	result, err := service.Grant(carrier, ports.GrantCommand{
		UserID:   4201,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("carrier-grant-commit", "carrier-grant-commit"),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Grant() on carrier error = %v", err)
	}
	// Same-transaction visibility: the ledger row must be readable from
	// INSIDE the still-open carrier (a post-commit dual-write could
	// never satisfy this).
	if got := countLedgerByTransactionID(t, carrier, tx, result.TransactionID); got != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("ledger rows visible inside carrier = %d, want 1", got)
	}
	// ... and invisible outside it before the carrier commits.
	if got := countLedgerByTransactionID(t, ctx, pool, result.TransactionID); got != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("ledger rows visible on pool before commit = %d, want 0", got)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	wallet, err := service.GetWallet(ctx, 4201)
	if err != nil {
		t.Fatalf("GetWallet() after carrier commit error = %v", err)
	}
	if wallet.Available != 100 {
		t.Fatalf("wallet available after carrier commit = %d, want 100", wallet.Available)
	}
}

func TestGrantRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	service := NewService(pool)
	ctx := context.Background()
	testhelpers.EnsureAdminOperationUser(t, pool, 4202)

	tx, carrier := beginCarrier(t, ctx, pool)
	result, err := service.Grant(carrier, ports.GrantCommand{
		UserID:   4202,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("carrier-grant-rollback", "carrier-grant-rollback"),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Grant() on carrier error = %v", err)
	}
	// The business fails after the domain write: the guard rolls the
	// whole carrier back, domain write included. Step 2 could not do
	// this (the domain committed in its own transaction first).
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if got := countLedgerByTransactionID(t, ctx, pool, result.TransactionID); got != 0 {
		t.Fatalf("ledger rows after carrier rollback = %d, want 0", got)
	}
	// No grant survived the rollback: the wallet holds nothing.
	wallet, err := service.GetWallet(ctx, 4202)
	if err != nil {
		t.Fatalf("GetWallet() after carrier rollback error = %v", err)
	}
	if wallet.Available != 0 || wallet.Reserved != 0 {
		t.Fatalf("wallet after carrier rollback = available %d reserved %d, want 0/0",
			wallet.Available, wallet.Reserved)
	}
}

func TestRestrictWalletJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID = 4203
	testhelpers.EnsureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("carrier-restrict:grant", "carrier-restrict-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	tx, carrier := beginCarrier(t, ctx, pool)
	change, err := service.RestrictWallet(carrier, ports.RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: domain.BusinessReference{Type: "finding", ID: "carrier-finding-a"},
		Reason:        "projection mismatch",
		Actor:         domain.Actor{Type: domain.ActorSystem, ID: "reconciliation"},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("RestrictWallet() on carrier error = %v", err)
	}
	// Same-transaction visibility for the restriction row.
	var inside int
	if err := tx.QueryRow(carrier,
		`SELECT count(*) FROM credit_wallet_restrictions WHERE restriction_id = $1::uuid AND released_at IS NULL`,
		change.RestrictionID).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count restrictions inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("restrictions visible inside carrier = %d, want 1", inside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() after carrier commit error = %v", err)
	}
	if len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("active restrictions after carrier commit = %d, want 1", len(wallet.ActiveRestrictions))
	}
}

func TestReleaseWalletRestrictionRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	service := NewService(pool)
	ctx := context.Background()
	const userID = 4204
	testhelpers.EnsureAdminOperationUser(t, pool, userID)
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 100),
		Metadata: testhelpers.TestMetadata("carrier-release:grant", "carrier-release-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	change, err := service.RestrictWallet(ctx, ports.RestrictWalletCommand{
		UserID:        userID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: domain.BusinessReference{Type: "finding", ID: "carrier-finding-b"},
		Reason:        "projection mismatch",
		Actor:         domain.Actor{Type: domain.ActorSystem, ID: "reconciliation"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}

	tx, carrier := beginCarrier(t, ctx, pool)
	if _, err := service.ReleaseWalletRestriction(carrier, ports.ReleaseWalletRestrictionCommand{
		RestrictionID: change.RestrictionID,
		Reason:        "projection reconciled",
		Actor:         domain.Actor{Type: domain.ActorSystem, ID: "reconciliation"},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ReleaseWalletRestriction() on carrier error = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	// The release rolled back with the carrier: the wallet is still
	// restricted afterwards.
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() after carrier rollback error = %v", err)
	}
	if len(wallet.ActiveRestrictions) != 1 {
		t.Fatalf("active restrictions after carrier rollback = %d, want 1 (release did not survive)", len(wallet.ActiveRestrictions))
	}
}
