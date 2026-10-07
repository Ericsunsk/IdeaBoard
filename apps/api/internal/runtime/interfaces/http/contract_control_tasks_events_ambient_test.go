package httpapi

// Control cancel post-commit publication over the guard-owned (ambient)
// transaction (#889).
//
// The defect these pin: on the ambient path the settlement joins the
// guard's transaction (which commits later, together with the audit
// row), but the cancelled transition was announced inside the handler —
// before the guard committed. An audit failure or a commit failure
// rolled the flip back AFTER both delivery channels had already
// announced a cancellation that never happened.
//
// The contract proven here, over the real settlement legs
// (PostgreSQL; skipped without API_TEST_DATABASE_DSN):
//   - adapter level: Cancel on a registry carrier settles silently
//     (nothing published, row still pre-commit outside); the
//     announcement appears only after carrier commit PLUS the
//     post-commit hooks run; a rollback without hooks stays silent;
//     a terminal replay registers nothing and stays silent.
//   - guard level (full HTTP through runAtomic): success publishes
//     exactly once per channel after commit; audit failure and commit
//     failure render 503, roll the flip back, and publish nothing; an
//     idempotent replay publishes nothing new.
// No second publisher is introduced: the same ControlTaskEventPort and
// the same two channels carry every announcement.

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/dbutil"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ambientPGTxScope is a guard TxScope over a real pool: Begin stages a
// real guard-owned transaction on the request context, Commit can be
// forced to fail to prove the commit-failure leg, Rollback discards.
type ambientPGTxScope struct {
	mu        sync.Mutex
	pool      *pgxpool.Pool
	begins    int
	commits   int
	rollbacks int
	commitErr error
}

func (s *ambientPGTxScope) Begin(ctx context.Context) (context.Context, error) {
	s.mu.Lock()
	s.begins++
	s.mu.Unlock()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	return ContextWithControlTx(ctx, tx), nil
}

func (s *ambientPGTxScope) Commit(ctx context.Context) error {
	s.mu.Lock()
	s.commits++
	commitErr := s.commitErr
	s.mu.Unlock()
	tx, ok := ControlTxFromContext(ctx)
	if !ok || tx == nil {
		return errControlAtomicDown
	}
	if commitErr != nil {
		_ = tx.Rollback(ctx)
		return commitErr
	}
	return tx.Commit(ctx)
}

func (s *ambientPGTxScope) Rollback(ctx context.Context) {
	s.mu.Lock()
	s.rollbacks++
	s.mu.Unlock()
	if tx, ok := ControlTxFromContext(ctx); ok && tx != nil {
		_ = tx.Rollback(ctx)
	}
}

func (s *ambientPGTxScope) counts() (begins, commits, rollbacks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.begins, s.commits, s.rollbacks
}

// beginAmbientCarrier stages a guard-style carrier for direct adapter
// calls: a real pool transaction plus the post-commit registry the
// guard attaches after its Begin. The caller owns commit/rollback and,
// on the success path, the post-commit hook run — mirroring runAtomic.
func beginAmbientCarrier(t *testing.T, pool *pgxpool.Pool) (pgx.Tx, context.Context) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin ambient carrier: %v", err)
	}
	return tx, dbutil.ContextWithAmbientPostCommit(ContextWithControlTx(ctx, tx))
}

func assertControlPortSilent(t *testing.T, events *recordingControlTaskEvents) {
	t.Helper()
	statuses, envelopes := events.snapshot()
	if len(statuses) != 0 || len(envelopes) != 0 {
		t.Fatalf("ambient cancel announced statuses=%d envelopes=%d, want 0/0 (nothing may publish before commit): %+v %+v",
			len(statuses), len(envelopes), statuses, envelopes)
	}
}

func assertControlPortAnnouncedOnce(t *testing.T, env *controlCancelEnv, events *recordingControlTaskEvents, taskID, reason, operator string) {
	t.Helper()
	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 {
		t.Fatalf("task status announcements = %d, want exactly 1: %+v", len(statuses), statuses)
	}
	if len(envelopes) != 1 {
		t.Fatalf("workspace envelopes = %d, want exactly 1: %+v", len(envelopes), envelopes)
	}
	published := statuses[0]
	if published.Status != string(workspacetasks.StatusCancelled) {
		t.Fatalf("status announcement = %q, want cancelled", published.Status)
	}
	if published.TaskID != taskID || published.WorkspaceID != env.workspace {
		t.Fatalf("announcement scope = %s/%s, want %s/%s", published.WorkspaceID, published.TaskID, env.workspace, taskID)
	}
	if published.Reason != reason {
		t.Fatalf("announcement reason = %q, want %q", published.Reason, reason)
	}
	if published.ActorUserID != operator {
		t.Fatalf("announcement actor = %q, want %q", published.ActorUserID, operator)
	}
	if published.OccurredAt.IsZero() {
		t.Fatal("announcement carried a zero occurred_at; it must be stamped post-commit")
	}
	if envelopes[0].TaskID != taskID || envelopes[0].Status != string(workspacetasks.StatusCancelled) {
		t.Fatalf("workspace envelope = %+v, want task.cancelled for %s", envelopes[0], taskID)
	}
}

// TestControlAdminCancelAmbientSettlesSilentlyUntilPostCommit is the
// adapter-level deferral proof: Cancel on the registry carrier flips
// inside the still-open transaction but announces nothing and leaves
// the row pre-commit outside; commit plus the hook run publishes
// exactly once per channel and the flip turns durable.
func TestControlAdminCancelAmbientSettlesSilentlyUntilPostCommit(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_defer1"
	const reason = "ambient operator cancel"
	env.seedPaidTask(t, creditService, taskID)

	operatorPublicID := uuid.New()
	operator := tenancy.FormatUserPublicID(operatorPublicID)
	events := &recordingControlTaskEvents{}
	adapter := env.adapter(
		&controlTxHolder{credits: creditService},
		controlOperatorDirectory{userID: env.userID, publicID: operatorPublicID},
		events,
	)

	tx, carrier := beginAmbientCarrier(t, env.pool)
	if err := adapter.CancelAdminTask(carrier, taskID, env.userID, reason); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("CancelAdminTask on carrier: %v", err)
	}
	// Settled but uncommitted: silent on both channels, invisible outside.
	assertControlPortSilent(t, events)
	if got := env.taskStatus(t, taskID); got != "pending" {
		_ = tx.Rollback(context.Background())
		t.Fatalf("task status before carrier commit = %q, want pending", got)
	}
	// The guard commits, then runs the hooks: exactly-once announcement.
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit ambient carrier: %v", err)
	}
	dbutil.RunAmbientPostCommit(carrier)
	assertControlPortAnnouncedOnce(t, env, events, taskID, reason, operator)
	if got := env.taskStatus(t, taskID); got != "cancelled" {
		t.Fatalf("task status after carrier commit = %q, want cancelled", got)
	}
}

// TestControlAdminCancelAmbientRollbackAnnouncesNothing proves the
// discard half at the adapter seam: the carrier rolls back without the
// hooks ever running, so the deferred transition is dropped — the task
// stays pending and both channels stay silent.
func TestControlAdminCancelAmbientRollbackAnnouncesNothing(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_rollback1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{}
	adapter := env.adapter(&controlTxHolder{credits: creditService}, nil, events)

	tx, carrier := beginAmbientCarrier(t, env.pool)
	if err := adapter.CancelAdminTask(carrier, taskID, env.userID, "should roll back"); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("CancelAdminTask on carrier: %v", err)
	}
	// Guard-side failure: rollback, hooks never run.
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback ambient carrier: %v", err)
	}
	assertControlPortSilent(t, events)
	if got := env.taskStatus(t, taskID); got != "pending" {
		t.Fatalf("task status after carrier rollback = %q, want pending", got)
	}
}

// TestControlAdminCancelAmbientReplayAnnouncesNothing proves exactly-once
// across terminal replays on the ambient path: the second cancel
// registers no hooks, so commit plus hook run announces nothing new.
func TestControlAdminCancelAmbientReplayAnnouncesNothing(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_replay1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{}
	adapter := env.adapter(&controlTxHolder{credits: creditService}, nil, events)

	tx, carrier := beginAmbientCarrier(t, env.pool)
	if err := adapter.CancelAdminTask(carrier, taskID, env.userID, "first"); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("first CancelAdminTask: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit ambient carrier: %v", err)
	}
	dbutil.RunAmbientPostCommit(carrier)
	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("after first cancel: statuses=%d envelopes=%d, want 1/1", len(statuses), len(envelopes))
	}

	// The re-cancel hits an already-terminal row on a fresh carrier.
	tx2, carrier2 := beginAmbientCarrier(t, env.pool)
	if err := adapter.CancelAdminTask(carrier2, taskID, env.userID, "replay"); err != nil {
		_ = tx2.Rollback(context.Background())
		t.Fatalf("replayed CancelAdminTask: %v", err)
	}
	if err := tx2.Commit(context.Background()); err != nil {
		t.Fatalf("commit replay carrier: %v", err)
	}
	dbutil.RunAmbientPostCommit(carrier2)
	statuses, envelopes = events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("after replay: statuses=%d envelopes=%d, want still 1/1 (a replay announces nothing)", len(statuses), len(envelopes))
	}
}

// ambientGuardHarness wires the REAL cancel adapter (real store, real
// credits holder, recording port) behind the REAL guard atomic path,
// so the tests below drive the exact production sequence: guard Begin
// → handler settlement joining the carrier → audit join → commit →
// post-commit hook run → response.
func ambientGuardHarness(t *testing.T, env *controlCancelEnv, creditService *credits.Service, scope *ambientPGTxScope, audit *stubControlTxAudit, events *recordingControlTaskEvents, operatorPublicID uuid.UUID) *controlHarness {
	adapter := NewWorkspaceControlTasks(env.pool, taskpostgres.NewStore(env.pool),
		&controlTxHolder{credits: creditService},
		controlOperatorDirectory{userID: controlOperatorID, publicID: operatorPublicID},
		events)
	return newControlHarness(t, func(d *ContractControlDeps) {
		d.Tasks = adapter
		d.TxScope = scope
		d.TxAudit = audit
	})
}

func serveAmbientCancel(t *testing.T, h *controlHarness, taskID, key, reason string) (int, map[string]any) {
	t.Helper()
	status, _, payload := h.serve(controlCall{
		method: http.MethodPost, path: "/api/control/v1/tasks/" + taskID + "/cancel",
		userID: controlOperatorID, role: "admin",
		headers: controlMutationHeaders(key, reason),
	})
	return status, payload
}

// TestControlGuardCancelAmbientSuccessPublishesAfterCommit drives the
// full production path: the announcement lands exactly once per
// channel only after the guard commits business plus audit together.
func TestControlGuardCancelAmbientSuccessPublishesAfterCommit(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_guard_ok1"
	const reason = "ambient guard cancel"
	env.seedPaidTask(t, creditService, taskID)

	operatorPublicID := uuid.New()
	operator := tenancy.FormatUserPublicID(operatorPublicID)
	events := &recordingControlTaskEvents{}
	scope := &ambientPGTxScope{pool: env.pool}
	audit := &stubControlTxAudit{durable: true}
	h := ambientGuardHarness(t, env, creditService, scope, audit, events, operatorPublicID)

	if status, payload := serveAmbientCancel(t, h, taskID, "amb-ok-1", reason); status != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200: %v", status, payload)
	}
	assertControlPortAnnouncedOnce(t, env, events, taskID, reason, operator)
	if got := env.taskStatus(t, taskID); got != "cancelled" {
		t.Fatalf("task status = %q, want cancelled (business and audit committed together)", got)
	}
	if begins, commits, rollbacks := scope.counts(); begins != 1 || commits != 1 || rollbacks != 0 {
		t.Fatalf("guard tx boundaries = %d/%d/%d, want 1/1/0", begins, commits, rollbacks)
	}
	if _, ok := audit.last(); !ok {
		t.Fatal("audit row missing: the success audit must join the committed transaction")
	}
}

// TestControlGuardCancelAmbientAuditFailureDiscards is the audit-failure
// leg: the audit row cannot join, so the whole unit — flip, release,
// and the deferred announcement — rolls back and stays silent.
func TestControlGuardCancelAmbientAuditFailureDiscards(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_guard_audit1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{}
	scope := &ambientPGTxScope{pool: env.pool}
	audit := &stubControlTxAudit{durable: true, err: errControlAtomicDown}
	h := ambientGuardHarness(t, env, creditService, scope, audit, events, uuid.New())

	if status, _ := serveAmbientCancel(t, h, taskID, "amb-audit-1", "audit is down"); status != http.StatusServiceUnavailable {
		t.Fatalf("cancel status = %d, want 503 (audit failure rolls the call back)", status)
	}
	assertControlPortSilent(t, events)
	if got := env.taskStatus(t, taskID); got != "pending" {
		t.Fatalf("task status = %q, want pending (the flip rolled back with the audit)", got)
	}
	if _, _, rollbacks := scope.counts(); rollbacks != 1 {
		t.Fatalf("guard rollbacks = %d, want 1", rollbacks)
	}
}

// TestControlGuardCancelAmbientCommitFailureDiscards is the commit-failure
// leg: the commit itself rejects, so the deferred announcement (which
// only runs after a successful commit) never fires.
func TestControlGuardCancelAmbientCommitFailureDiscards(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_guard_commit1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{}
	scope := &ambientPGTxScope{pool: env.pool, commitErr: errControlAtomicDown}
	audit := &stubControlTxAudit{durable: true}
	h := ambientGuardHarness(t, env, creditService, scope, audit, events, uuid.New())

	if status, _ := serveAmbientCancel(t, h, taskID, "amb-commit-1", "commit is down"); status != http.StatusServiceUnavailable {
		t.Fatalf("cancel status = %d, want 503 (commit failure fails the call)", status)
	}
	assertControlPortSilent(t, events)
	if got := env.taskStatus(t, taskID); got != "pending" {
		t.Fatalf("task status = %q, want pending (the failed commit landed nothing)", got)
	}
}

// TestControlGuardCancelAmbientReplayStaysSilent proves idempotent
// re-cancel through the guard announces nothing new: the replay never
// runs the handler, so no hooks exist to run after its commit.
func TestControlGuardCancelAmbientReplayStaysSilent(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_amb_guard_replay1"
	const reason = "ambient guard replay"
	env.seedPaidTask(t, creditService, taskID)

	operatorPublicID := uuid.New()
	operator := tenancy.FormatUserPublicID(operatorPublicID)
	events := &recordingControlTaskEvents{}
	scope := &ambientPGTxScope{pool: env.pool}
	audit := &stubControlTxAudit{durable: true}
	h := ambientGuardHarness(t, env, creditService, scope, audit, events, operatorPublicID)

	if status, _ := serveAmbientCancel(t, h, taskID, "amb-replay-1", reason); status != http.StatusOK {
		t.Fatalf("first cancel status = %d, want 200", status)
	}
	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("after first cancel: statuses=%d envelopes=%d, want 1/1", len(statuses), len(envelopes))
	}
	// Same key plus same body: the guard replays without running the
	// handler, so the committed replay audit commits but no hooks run.
	if status, _ := serveAmbientCancel(t, h, taskID, "amb-replay-1", reason); status != http.StatusOK {
		t.Fatalf("replay cancel status = %d, want 200", status)
	}
	assertControlPortAnnouncedOnce(t, env, events, taskID, reason, operator)
}
