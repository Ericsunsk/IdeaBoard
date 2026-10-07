package idempotency

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testControlClaim(key, hash string) ControlClaim {
	return ControlClaim{
		Operator:    "usr_testoperator",
		Operation:   "users.ban",
		TargetType:  "user",
		TargetID:    "usr_testtarget",
		Key:         key,
		RequestHash: hash,
	}
}

const testControlHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testControlHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func openControlStore(t *testing.T) (*ControlStore, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Open(t)
	return NewControlStore(pool), pool
}

func TestControlClaimWinnerCompletesAndReplays(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("winner-key-1", testControlHashA)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	result, err := store.Claim(ctx, tx, claim)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim: %v", err)
	}
	if !result.Winner || result.Existing != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("first claim = %+v, want winner", result)
	}
	stored := ControlResult{Status: 200, ContentType: "application/json; charset=utf-8", Body: `{"data":{"banned":true}}`}
	if err := store.Complete(ctx, tx, claim, stored); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Complete: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin replay: %v", err)
	}
	defer tx2.Rollback(ctx)
	replay, err := store.Claim(ctx, tx2, claim)
	if err != nil {
		t.Fatalf("replay Claim: %v", err)
	}
	if replay.Winner || replay.Existing == nil {
		t.Fatalf("replay claim = %+v, want loser with existing", replay)
	}
	if replay.Existing.RequestHash != testControlHashA || !replay.Existing.Completed || replay.Existing.Expired {
		t.Fatalf("existing = %+v, want completed unexpired same-hash", replay.Existing)
	}
	if replay.Existing.Result != stored {
		t.Fatalf("replay result = %+v, want %+v", replay.Existing.Result, stored)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("replay commit: %v", err)
	}
}

func TestControlClaimConflictOnDifferentHash(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("conflict-key-1", testControlHashA)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Claim(ctx, tx, claim); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim: %v", err)
	}
	if err := store.Complete(ctx, tx, claim, ControlResult{Status: 200, ContentType: "application/json", Body: "{}"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Complete: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin conflict: %v", err)
	}
	defer tx2.Rollback(ctx)
	divergent := claim
	divergent.RequestHash = testControlHashB
	loser, err := store.Claim(ctx, tx2, divergent)
	if err != nil {
		t.Fatalf("conflict Claim: %v", err)
	}
	if loser.Winner || loser.Existing == nil {
		t.Fatalf("conflict claim = %+v, want loser with existing", loser)
	}
	if loser.Existing.RequestHash != testControlHashA {
		t.Fatalf("existing hash = %q, want the first request hash", loser.Existing.RequestHash)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("conflict commit: %v", err)
	}
}

func TestControlClaimAbandonLetsRetryReExecute(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("abandon-key-1", testControlHashA)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Claim(ctx, tx, claim); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim: %v", err)
	}
	// Business failure: abandon the claim in the same transaction, then
	// commit (the failure audit row commits alongside in the guard).
	if err := store.Abandon(ctx, tx, claim); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Abandon: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin retry: %v", err)
	}
	defer tx2.Rollback(ctx)
	retry, err := store.Claim(ctx, tx2, claim)
	if err != nil {
		t.Fatalf("retry Claim: %v", err)
	}
	if !retry.Winner {
		t.Fatalf("retry claim = %+v, want winner after abandon", retry)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("retry commit: %v", err)
	}
}

func TestControlClaimRollbackLeavesNoFact(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("rollback-key-1", testControlHashA)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Claim(ctx, tx, claim); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim: %v", err)
	}
	// Crash window: the guard transaction rolls back (audit join or
	// commit failure). The uncommitted claim disappears with it.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin retry: %v", err)
	}
	defer tx2.Rollback(ctx)
	retry, err := store.Claim(ctx, tx2, claim)
	if err != nil {
		t.Fatalf("retry Claim: %v", err)
	}
	if !retry.Winner {
		t.Fatalf("retry claim = %+v, want winner after rollback (no orphan lease)", retry)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("retry commit: %v", err)
	}
}

func TestControlClaimConcurrentSameKeySingleWinner(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("concurrent-key-1", testControlHashA)

	const callers = 4
	winners := make(chan bool, callers)
	records := make(chan *ControlRecord, callers)
	errs := make(chan error, callers)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(ctx)
			result, err := store.Claim(ctx, tx, claim)
			if err != nil {
				errs <- err
				return
			}
			if result.Winner {
				// Hold the transaction open so every loser blocks on
				// the primary key, then completes and commits.
				<-release
				if err := store.Complete(ctx, tx, claim, ControlResult{Status: 200, ContentType: "application/json", Body: `{"data":{"ok":true}}`}); err != nil {
					errs <- err
					return
				}
				if err := tx.Commit(ctx); err != nil {
					errs <- err
					return
				}
				winners <- true
				return
			}
			winners <- false
			records <- result.Existing
			if err := tx.Commit(ctx); err != nil {
				errs <- err
			}
		}()
	}
	// Let every claimant reach the INSERT (losers block), then release
	// the winner.
	time.Sleep(500 * time.Millisecond)
	close(release)
	wg.Wait()
	close(winners)
	close(records)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent claim error: %v", err)
	}
	winnerCount := 0
	for w := range winners {
		if w {
			winnerCount++
		}
	}
	if winnerCount != 1 {
		t.Fatalf("winners = %d, want exactly 1", winnerCount)
	}
	for record := range records {
		if record == nil || !record.Completed || record.RequestHash != testControlHashA {
			t.Fatalf("loser record = %+v, want completed same-hash", record)
		}
		if record.Result.Body != `{"data":{"ok":true}}` {
			t.Fatalf("loser result body = %q", record.Result.Body)
		}
	}
}

func TestControlClaimExpiredCompletedReadsAsMiss(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claim := testControlClaim("expiry-key-1", testControlHashA)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Claim(ctx, tx, claim); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim: %v", err)
	}
	if err := store.Complete(ctx, tx, claim, ControlResult{Status: 200, ContentType: "application/json", Body: "{}"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Complete: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// Age the fact past retention directly: expiry is a store-level
	// concern, not clock injection in the guard.
	if _, err := pool.Exec(ctx, `UPDATE control_idempotency_records SET expires_at = now() - INTERVAL '1 second'
		WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5`,
		claim.Operator, claim.Operation, claim.TargetType, claim.TargetID, claim.Key); err != nil {
		t.Fatalf("expire fact: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin takeover: %v", err)
	}
	defer tx2.Rollback(ctx)
	takeover, err := store.Claim(ctx, tx2, claim)
	if err != nil {
		t.Fatalf("takeover Claim: %v", err)
	}
	if !takeover.Winner {
		t.Fatalf("takeover claim = %+v, want winner (expiry reads as miss)", takeover)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("takeover commit: %v", err)
	}
}

func TestControlPurgeExpired(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()

	keep := testControlClaim("purge-keep-1", testControlHashA)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Claim(ctx, tx, keep); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim keep: %v", err)
	}
	if err := store.Complete(ctx, tx, keep, ControlResult{Status: 200, ContentType: "application/json", Body: "{}"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Complete keep: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit keep: %v", err)
	}

	stale := testControlClaim("purge-stale-1", testControlHashA)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin stale: %v", err)
	}
	if _, err := store.Claim(ctx, tx, stale); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Claim stale: %v", err)
	}
	if err := store.Complete(ctx, tx, stale, ControlResult{Status: 200, ContentType: "application/json", Body: "{}"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Complete stale: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit stale: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE control_idempotency_records SET expires_at = now() - INTERVAL '1 second'
		WHERE idempotency_key = 'purge-stale-1'`); err != nil {
		t.Fatalf("expire stale: %v", err)
	}

	purged, err := store.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1 (only the expired fact)", purged)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_idempotency_records WHERE idempotency_key = 'purge-keep-1'`).Scan(&remaining); err != nil {
		t.Fatalf("count keep: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("kept rows = %d, want 1", remaining)
	}
}

func TestControlStoreFailClosed(t *testing.T) {
	ctx := context.Background()
	var nilStore *ControlStore
	if _, err := nilStore.Claim(ctx, nil, testControlClaim("x", testControlHashA)); err == nil {
		t.Fatal("nil store Claim must fail closed, never execute unfenced")
	}
	unwired := NewControlStore(nil)
	if _, err := unwired.Claim(ctx, nil, testControlClaim("x", testControlHashA)); err == nil {
		t.Fatal("unwired store Claim must fail closed")
	}
	if _, err := unwired.PurgeExpired(ctx); err == nil {
		t.Fatal("unwired store PurgeExpired must fail closed")
	}

	store, pool := openControlStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.Claim(ctx, nil, testControlClaim("x", testControlHashA)); err == nil {
		t.Fatal("nil transaction Claim must fail closed")
	}
	if err := store.Complete(ctx, tx, testControlClaim("x", testControlHashA), ControlResult{Status: 500}); err == nil {
		t.Fatal("non-2xx Complete must be rejected (failures are never retained)")
	}
}

func TestControlClaimKeyIncludesTarget(t *testing.T) {
	store, pool := openControlStore(t)
	ctx := context.Background()
	claimA := testControlClaim("target-key-1", testControlHashA)
	claimA.TargetID = "usr_targeta"
	claimB := testControlClaim("target-key-1", testControlHashA)
	claimB.TargetID = "usr_targetb"

	// Same key, same body, different target: independent durable scopes,
	// so each wins its own fact. The guard still renders 409 for this
	// shape because its request hash pins the target; the store proves
	// the scope separation underneath.
	for i, claim := range []ControlClaim{claimA, claimB} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		result, err := store.Claim(ctx, tx, claim)
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Claim %d: %v", i, err)
		}
		if !result.Winner {
			_ = tx.Rollback(ctx)
			t.Fatalf("claim %d = %+v, want winner (target scopes separate)", i, result)
		}
		if err := store.Complete(ctx, tx, claim, ControlResult{Status: 200, ContentType: "application/json", Body: fmt.Sprintf(`{"n":%d}`, i)}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Complete %d: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_idempotency_records WHERE idempotency_key = 'target-key-1'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("facts = %d, want 2 (one per target scope)", count)
	}
}
