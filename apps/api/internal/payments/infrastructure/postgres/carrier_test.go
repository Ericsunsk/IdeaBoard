package postgres

// Control audit atomicity, step 3: the payments repository joins the
// ambient guard transaction instead of committing in its own.
//
// What is proven here, against the isolated fixture database:
//   - paymentContextDB.withTx (the choke behind RecordPaidPaymentOrder
//     and CreditPaidPaymentOrder) runs its unit on the carrier: the
//     write is readable from the still-open carrier, invisible on the
//     pool, then durable after the carrier commits, and the repository
//     performs no commit of its own.
//   - A carrier rollback leaves no payments artifact behind (the
//     step-2 gap: the payments commit used to survive on its own).
//   - beginDirectTx (the admin refund paths) borrows the carrier when
//     staged and opens an owned pool transaction otherwise.
//   - With no carrier the starter paths run unchanged (the existing
//     payments suite is that proof; it is not duplicated here).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func paymentsCarrierProbeTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	table := fmt.Sprintf("pay_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})
	return table
}

func TestPaymentsWithTxJoinsAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	table := paymentsCarrierProbeTable(t, ctx, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	// The unit runs on the carrier and the repository commits nothing
	// itself: a nil error here must still leave the write uncommitted.
	if err := repo.dbWithContext(carrier).withTx(func(_ *dbsqlc.Queries, gotTx pgx.Tx) error {
		if gotTx == nil {
			return fmt.Errorf("payments withTx ran without a transaction")
		}
		_, err := gotTx.Exec(carrier, `INSERT INTO `+table+` (id, note) VALUES ('pay-1', 'converged')`)
		return err
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("withTx on carrier error = %v", err)
	}
	// Same-transaction visibility ...
	var inside int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM `+table+` WHERE id = 'pay-1'`).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("payments rows visible inside carrier = %d, want 1", inside)
	}
	// ... invisible outside before the carrier commits (the repository
	// committed nothing of its own).
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = 'pay-1'`).Scan(&outside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe on pool: %v", err)
	}
	if outside != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("payments rows visible on pool before commit = %d, want 0", outside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = 'pay-1'`).Scan(&outside); err != nil {
		t.Fatalf("count probe after commit: %v", err)
	}
	if outside != 1 {
		t.Fatalf("payments rows after carrier commit = %d, want 1", outside)
	}
}

func TestPaymentsWithTxRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	table := paymentsCarrierProbeTable(t, ctx, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := repo.dbWithContext(carrier).withTx(func(_ *dbsqlc.Queries, gotTx pgx.Tx) error {
		_, err := gotTx.Exec(carrier, `INSERT INTO `+table+` (id, note) VALUES ('pay-2', 'doomed')`)
		return err
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("withTx on carrier error = %v", err)
	}
	// The business fails after the payments write: the guard rolls the
	// whole carrier back, payments write included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = 'pay-2'`).Scan(&outside); err != nil {
		t.Fatalf("count probe after rollback: %v", err)
	}
	if outside != 0 {
		t.Fatalf("payments rows after carrier rollback = %d, want 0", outside)
	}
}

func TestBeginDirectTxBorrowsCarrierWhenStaged(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := NewRepository(pool)

	// No carrier: an owned pool transaction, exactly as before.
	ownedTx, owned, err := repo.beginDirectTx(ctx)
	if err != nil {
		t.Fatalf("beginDirectTx without carrier error = %v", err)
	}
	if !owned || ownedTx == nil {
		t.Fatal("beginDirectTx without carrier must open an owned transaction")
	}
	_ = ownedTx.Rollback(ctx)

	// Carrier staged: the same carrier transaction, borrowed.
	carrierTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	defer func() { _ = carrierTx.Rollback(ctx) }()
	borrowed, borrowedOwned, err := repo.beginDirectTx(dbutil.ContextWithAmbientTx(ctx, carrierTx))
	if err != nil {
		t.Fatalf("beginDirectTx with carrier error = %v", err)
	}
	if borrowedOwned {
		t.Fatal("beginDirectTx with carrier must borrow, not own")
	}
	if borrowed == nil {
		t.Fatal("beginDirectTx with carrier returned nil")
	}
	// The borrowed handle observes the carrier's writes: proof it is the
	// same transaction, not a nested one.
	if _, err := carrierTx.Exec(ctx, `CREATE TEMP TABLE pay_carrier_identity (id int PRIMARY KEY)`); err != nil {
		t.Fatalf("stage identity marker: %v", err)
	}
	var marker int
	if err := borrowed.QueryRow(ctx, `SELECT count(*) FROM pay_carrier_identity`).Scan(&marker); err != nil {
		t.Fatalf("borrowed tx cannot see the carrier session state (not the same tx): %v", err)
	}
}
