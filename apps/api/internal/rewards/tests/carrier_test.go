package tests

// Control audit atomicity, step 3: the reward-codes admin writes join
// the ambient guard transaction instead of committing in their own.
//
// What is proven here, against the isolated fixture database:
//   - CreateRewardCode on a carrier context lands inside the carrier
//     transaction: the definition and journal rows are readable from
//     the still-open carrier and invisible on the pool, then durable
//     after the carrier commits (an owned commit would already be
//     visible on the pool, so the pair proves no new transaction was
//     opened).
//   - A carrier rollback leaves no definition row and no journal row.
//   - An idempotent replay on a carrier context does NOT release the
//     borrowed transaction: the carrier stays usable and commits
//     exactly one definition with one journal row.
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing code service suite is that proof; it is not duplicated
//     here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rewardCarrierQuerier is satisfied by both pgx.Tx (same-transaction
// reads) and *pgxpool.Pool (outside reads) for the visibility
// assertions below.
type rewardCarrierQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func beginRewardCarrier(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, context.Context) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	return tx, dbutil.ContextWithAmbientTx(ctx, tx)
}

func countRewardCodeRows(t *testing.T, ctx context.Context, querier rewardCarrierQuerier, displayName string) (definitions, actions int) {
	t.Helper()
	if err := querier.QueryRow(ctx, `SELECT
  (SELECT COUNT(*) FROM reward_codes WHERE display_name = $1),
  (SELECT COUNT(*) FROM reward_admin_actions WHERE action_type = 'create_reward_code'
     AND target_snapshot->>'display_name' = $1)`, displayName).Scan(&definitions, &actions); err != nil {
		t.Fatalf("count reward Code rows: %v", err)
	}
	return definitions, actions
}

func TestRewardCodeCreateJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	ctx := context.Background()

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	created, err := service.CreateRewardCode(carrier, ports.CreateRewardCodeCommand{
		DisplayName: "carrier commit", Code: "CARRIER-JOIN-77", CreditAmount: 50,
		MaxRedemptions: 1, IdempotencyKey: "reward-carrier-commit-001", ActorUserID: 1,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateRewardCode() on carrier error = %v", err)
	}
	if created.Replayed || created.Plaintext == "" {
		_ = tx.Rollback(ctx)
		t.Fatalf("created = %+v, want fresh create with plaintext", created)
	}
	// Same-transaction visibility: both rows must be readable from
	// INSIDE the still-open carrier (a post-commit dual-write could
	// never satisfy this).
	if definitions, actions := countRewardCodeRows(t, carrier, tx, "carrier commit"); definitions != 1 || actions != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("rows inside carrier = definitions %d actions %d, want 1/1", definitions, actions)
	}
	// ... and invisible outside it before the carrier commits (an
	// owned commit would already be visible on the pool).
	if definitions, actions := countRewardCodeRows(t, ctx, pool, "carrier commit"); definitions != 0 || actions != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("rows on pool before commit = definitions %d actions %d, want 0/0", definitions, actions)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if definitions, actions := countRewardCodeRows(t, ctx, pool, "carrier commit"); definitions != 1 || actions != 1 {
		t.Fatalf("rows on pool after commit = definitions %d actions %d, want 1/1", definitions, actions)
	}
}

func TestRewardCodeCreateRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	ctx := context.Background()

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	if _, err := service.CreateRewardCode(carrier, ports.CreateRewardCodeCommand{
		DisplayName: "carrier rollback", Code: "CARRIER-ROLLBACK-78", CreditAmount: 50,
		MaxRedemptions: 1, IdempotencyKey: "reward-carrier-rollback-001", ActorUserID: 1,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("CreateRewardCode() on carrier error = %v", err)
	}
	// The guard rolls the whole carrier back, domain writes included.
	// Step 2 could not do this (the domain committed in its own
	// transaction first).
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if definitions, actions := countRewardCodeRows(t, ctx, pool, "carrier rollback"); definitions != 0 || actions != 0 {
		t.Fatalf("rows after carrier rollback = definitions %d actions %d, want 0/0", definitions, actions)
	}
}

func TestRewardCodeReplayDoesNotReleaseAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	service := newTestRewardService(t, pool, credits.NewService(pool))
	ctx := context.Background()

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	command := ports.CreateRewardCodeCommand{
		DisplayName: "carrier replay", Code: "CARRIER-REPLAY-79", CreditAmount: 50,
		MaxRedemptions: 1, IdempotencyKey: "reward-carrier-replay-001", ActorUserID: 1,
	}
	if _, err := service.CreateRewardCode(carrier, command); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("first CreateRewardCode() on carrier error = %v", err)
	}
	// The replay path must NOT roll the borrowed transaction back: an
	// owned-mode release here would discard the carrier (and the
	// guard's audit row with it).
	replay, err := service.CreateRewardCode(carrier, command)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("replayed CreateRewardCode() on carrier error = %v", err)
	}
	if !replay.Replayed || replay.Plaintext != "" {
		_ = tx.Rollback(ctx)
		t.Fatalf("replay = %+v, want replayed metadata without plaintext", replay)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx after replay: %v", err)
	}
	if definitions, actions := countRewardCodeRows(t, ctx, pool, "carrier replay"); definitions != 1 || actions != 1 {
		t.Fatalf("rows on pool after commit = definitions %d actions %d, want 1/1", definitions, actions)
	}
}
