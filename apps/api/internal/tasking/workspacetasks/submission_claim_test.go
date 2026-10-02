// Durable submit idempotency for task:create.
//
// Regression cover for the confirmed double-charge / double-image defect:
// the v1 edge's only dedup was the process-local
// platform/idempotency.Store, so a submit that committed and then died
// before the cache write (or lost its entry to the 4096-entry eviction)
// re-entered settlement with a freshly minted task id. The Credit Hold's
// ledger key was derived from that per-call id, so the ledger's
// pg_advisory_xact_lock and unique constraint could not fence it: the retry
// took a second hold, inserted a second task, and enqueued a second River
// job. Both tasks ran the provider and completed, so the user was charged
// twice and received two images for one 202.
//
// What is proven here, all against real PostgreSQL:
//   - Same key twice returns the SAME task: one task row, one hold, one
//     enqueue probe.
//   - Same key with a different body is a 409-class conflict and produces no
//     second task.
//   - The crash window itself: the durable path is exercised directly with
//     no process-local cache in the picture, and the second request still
//     replays the first task.
//   - Any failure inside the transaction rolls the durable claim back too,
//     so a failed submit never leaves an "accepted but no task" record that
//     would poison the key.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).
package workspacetasks_test

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// claimCount counts durable submit claims in the fixture schema.
func (e *atomicEnv) claimCount(t *testing.T, key string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM task_submission_idempotency c
		 INNER JOIN workspaces w ON w.internal_id = c.workspace_id
		 WHERE w.public_id = $1 AND c.operation = $2 AND c.idempotency_key = $3`,
		e.ws, workspacetasks.TaskSubmitOperation, key,
	).Scan(&count); err != nil {
		t.Fatalf("count submission claims: %v", err)
	}
	return count
}

func (e *atomicEnv) holdCount(t *testing.T, key string) int {
	t.Helper()
	return e.ledgerCount(t, `idempotency_key = $1`, workspacetasks.TaskSubmitHoldKey(e.ws, key))
}

func (e *atomicEnv) submit(t *testing.T, key string, input workspacetasks.CreateInput) (*workspacetasks.WorkspaceTask, error) {
	t.Helper()
	input.WorkspaceID = e.ws
	input.OwnerUserID = e.userID
	if input.Service == "" {
		input.Service = "render"
	}
	if input.Resolution == "" {
		input.Resolution = "1k"
	}
	return e.service.Create(context.Background(), input, key)
}

// TestSubmitSameKeyReplaysSameTask proves the durable claim fences a retried
// submit: the second request returns the FIRST task, and the database holds
// exactly one task row, one hold, and one enqueue probe. Before the fix this
// produced a second task, a second hold (double charge), and a second River
// job (double image).
func TestSubmitSameKeyReplaysSameTask(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "replay-key-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	second, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("retried submit must succeed by replaying, got: %v", err)
	}
	_ = env.service.StagedEvents()

	if second.ID != first.ID {
		t.Fatalf("replay returned task %s, want the original %s", second.ID, first.ID)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want exactly 1", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want exactly 1 (retried submit must not double-charge)", got)
	}
	if got := env.probeCount(t, first.ID); got != 1 {
		t.Fatalf("enqueue probes = %d, want exactly 1 (retried submit must not enqueue a second job)", got)
	}
	if got := env.claimCount(t, key); got != 1 {
		t.Fatalf("durable claims = %d, want exactly 1", got)
	}
	// The wallet is the billing truth: reserved must reflect ONE hold.
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10): a replay must not reserve again", available, reserved)
	}
	// A replay must not re-announce the task: the winning submit already
	// emitted task.created for it.
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("replay staged %d events, want 0 (task.created must not re-emit)", len(staged))
	}
}

// TestSubmitSameKeyDifferentBodyConflicts proves a key reused with a
// different request body is a conflict, not a replay of the wrong task and
// not a second settlement. This is the case the process-local store also
// guarded, but only while the process lived.
func TestSubmitSameKeyDifferentBodyConflicts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "conflict-key-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	// Same key, different body (quantity 2 bills differently).
	_, err = env.submit(t, key, workspacetasks.CreateInput{Quantity: 2})
	if !errors.Is(err, workspacetasks.ErrIdempotencyConflict) {
		t.Fatalf("same key + different body err = %v, want ErrIdempotencyConflict", err)
	}

	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want 1: a conflict must not create a second task", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want 1: a conflict must not charge again", got)
	}
	if _, err := env.store.GetTask(context.Background(), env.ws, first.ID); err != nil {
		t.Fatalf("original task must survive the conflict: %v", err)
	}
}

// TestSubmitReplaySurvivesLostProcessCache reproduces the crash window that
// made the original defect reachable: the first submit commits, then the
// caller "dies" before any process-local cache is written. The retry must
// still be fenced by the DURABLE claim.
//
// The property under test is precisely that the answer comes from the
// database, not from memory: this file never consults
// platform/idempotency.Store, and the fixture service is rebuilt from the
// same pool so nothing process-local can bridge the two calls.
func TestSubmitReplaySurvivesLostProcessCache(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "crash-window-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	// Simulate the process losing every in-memory replay cache entry: build a
	// brand-new Service over the same database. A cache-backed fix would
	// miss here; the durable claim must not.
	restarted := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.store, TaskBilling: env.store,
		Quoter: env.serviceDeps.Quoter, Holder: env.holder, Queue: env.queue,
		Resolver: allowTestPoolResolver(), SnapshotSaver: env.serviceDeps.SnapshotSaver,
	})

	second, err := restarted.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("submit after cache loss must replay from the durable claim, got: %v", err)
	}
	_ = restarted.StagedEvents()

	if second.ID != first.ID {
		t.Fatalf("post-crash replay returned %s, want the original %s", second.ID, first.ID)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want 1", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want 1: the crash window must not double-charge", got)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10)", available, reserved)
	}
}

// TestSubmitFailureRollsBackDurableClaim proves the claim is transactional,
// not a pre-write reservation. If settlement fails after the claim is taken,
// the claim must roll back with it — otherwise the key would be permanently
// poisoned with "accepted" but no task, and every retry would replay a task
// that does not exist.
func TestSubmitFailureRollsBackDurableClaim(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "rollback-key-" + randomAtomicHex(t, 4)

	// Fail the enqueue leg: the transaction is open and the claim is already
	// written when this trips.
	env.queue.failOnce(errors.New("river unavailable"))

	if _, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1}); err == nil {
		t.Fatal("submit with failing enqueue must fail")
	}

	if got := env.claimCount(t, key); got != 0 {
		t.Fatalf("durable claims = %d, want 0: the claim must roll back with the failed settlement", got)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 0 {
		t.Fatalf("task rows = %d (err %v), want 0", count, err)
	}
	if got := env.holdCount(t, key); got != 0 {
		t.Fatalf("hold ledger rows = %d, want 0", got)
	}

	// The key must still be usable: a rolled-back claim must not poison it.
	created, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("retry after rolled-back claim must succeed, got: %v", err)
	}
	_ = env.service.StagedEvents()
	if got := env.claimCount(t, key); got != 1 {
		t.Fatalf("durable claims after retry = %d, want 1", got)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows after retry = %d, want 1", got)
	}
	if created.ID == "" {
		t.Fatal("retry after rollback produced no task")
	}
}

// TestSubmitEmptyKeyFailsClosed proves the submit path cannot be unfenced.
// An empty key would mean "no fence at all", which is the defect class this
// seam removes, so it is rejected before any ledger, task, or queue write.
func TestSubmitEmptyKeyFailsClosed(t *testing.T) {
	env := openAtomicEnv(t, 10)

	if _, err := env.submit(t, "   ", workspacetasks.CreateInput{Quantity: 1}); err == nil {
		t.Fatal("empty idempotency key must be rejected, never settled unfenced")
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 0 {
		t.Fatalf("task rows = %d (err %v), want 0", count, err)
	}
	var claims int
	if err := env.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM task_submission_idempotency`).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable claims = %d, want 0", claims)
	}
}

// TestSubmitHoldKeyIsDurableNotTaskScoped pins the derivation rule that the
// original bug turned on: the Hold's ledger key must be a function of the
// client key, stable across attempts, and must NOT vary with the per-call
// task id (which is exactly what made a retry un-fenceable).
func TestSubmitHoldKeyIsDurableNotTaskScoped(t *testing.T) {
	key := TaskSubmitKeyFixture()
	first := workspacetasks.TaskSubmitHoldKey("ws_alpha", key)
	second := workspacetasks.TaskSubmitHoldKey("ws_alpha", key)

	if first != second {
		t.Fatalf("hold key is not stable for one client key: %q vs %q", first, second)
	}
	if other := workspacetasks.TaskSubmitHoldKey("ws_beta", key); other == first {
		t.Fatal("hold key must be workspace-scoped: two workspaces shared a ledger key")
	}
	if other := workspacetasks.TaskSubmitHoldKey("ws_alpha", key+"-other"); other == first {
		t.Fatal("hold key must vary with the client key")
	}
	// Must stay inside the ledger's 160-char bound.
	if len(first) > 160 {
		t.Fatalf("hold key length = %d, want <= 160 (ledger bound)", len(first))
	}
	if len(first) == len("task-hold:")+len("task_") {
		t.Fatal("hold key looks task-id derived")
	}
}

// TaskSubmitKeyFixture returns a stable opaque key for derivation assertions.
func TaskSubmitKeyFixture() string { return "client-key-fixture" }
