// Package workspacetasks_test proves the final-timeout settlement sweep: the
// convergence leg of the DESIGN.md contract that nothing else owned.
//
// The leak this closes (verified against the schema, not assumed): a worker
// SIGKILLed after its claim commits, whose River job later exhausts
// MaxAttempts and is discarded, leaves tasks.status='running',
// task_billing_settlements.state='held', and credit_holds.status='held'. The
// Credits stale-hold reconciler only INSERTs a finding for that shape
// (execute_credit_reconciliation), so without this sweep the user's Ideas stay
// locked permanently.
//
// Proven here, over the canonical fixture database:
//   - an abandoned hold is RELEASED and the task converges to failed (the leak
//     stops being permanent);
//   - a task a worker still holds is NOT touched (no false kill, no money error
//     on work that would have succeeded);
//   - repeating the sweep produces NO second Release;
//   - the sweep cannot settle across workspaces: a mis-paired (workspace, task)
//     converges nothing, because the terminal write is workspace-scoped.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).
package workspacetasks_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/tasking/workspacetasks"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
)

// ageTaskPastStallThreshold rewinds tasks.updated_at — the LIVENESS clock the
// sweep reads — so the row looks like no worker has held the lease in a full
// day. This is the exact durable shape of the leak (a claim that committed and
// was then never touched again).
//
// The trigger must be disabled first: set_tasks_updated_at is a BEFORE UPDATE
// trigger that unconditionally stamps NEW.updated_at = now(), so no UPDATE can
// ever back-date the row. That trigger is WHY updated_at is trustworthy as a
// liveness clock (no writer can forge a fresh timestamp for an abandoned
// lease), and also why a test has to reach past it to construct the shape.
func (e *atomicEnv) ageTaskPastStallThreshold(t *testing.T, taskID string, age time.Duration) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `ALTER TABLE tasks DISABLE TRIGGER set_tasks_updated_at`); err != nil {
		t.Fatalf("disable tasks updated_at trigger: %v", err)
	}
	t.Cleanup(func() {
		if _, err := e.pool.Exec(context.Background(), `ALTER TABLE tasks ENABLE TRIGGER set_tasks_updated_at`); err != nil {
			t.Errorf("re-enable tasks updated_at trigger: %v", err)
		}
	})
	if _, err := e.pool.Exec(ctx, `UPDATE tasks SET updated_at = $2 WHERE public_id = $1`,
		taskID, time.Now().Add(-age)); err != nil {
		t.Fatalf("age task %s: %v", taskID, err)
	}
	var aged time.Time
	if err := e.pool.QueryRow(ctx, `SELECT updated_at FROM tasks WHERE public_id = $1`, taskID).Scan(&aged); err != nil {
		t.Fatalf("read back aged task %s: %v", taskID, err)
	}
	if time.Since(aged) < age-time.Minute {
		t.Fatalf("task %s was not back-dated (updated_at %s, want ~%s old)", taskID, aged, age)
	}
}

// runStalledSweep runs one bounded sweep through the production seams: the
// postgres store discovers, and the real Service converges (single-transaction
// task flip + hold Release + settlement projection + staged event).
func (e *atomicEnv) runStalledSweep(t *testing.T, now time.Time) workspacetasks.StalledConvergence {
	t.Helper()
	report, err := workspacetasks.ConvergeStalledSettlements(context.Background(), workspacetasks.StalledConvergenceDeps{
		Scanner:       e.store,
		Failer:        e.service,
		StalledBefore: now.Add(-workspacetasks.StalledSettlementThreshold),
		Limit:         workspacetasks.StalledSettlementSweepBatch,
	})
	if err != nil {
		t.Fatalf("ConvergeStalledSettlements: %v (report %+v)", err, report)
	}
	return report
}

// claimTaskIn drives any workspace's task into the claimed (running) state the
// leak starts from.
func (e *atomicEnv) claimTaskIn(t *testing.T, workspaceID, taskID, token string) {
	t.Helper()
	if _, err := e.store.ClaimTask(context.Background(), workspaceID, taskID, token); err != nil {
		t.Fatalf("atomic ClaimTask(%s/%s): %v", workspaceID, taskID, err)
	}
}

func (e *atomicEnv) holdStatus(t *testing.T, taskID string) string {
	t.Helper()
	var holdID string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT hold_transaction_id FROM task_billing_snapshots WHERE task_id = $1`, taskID).Scan(&holdID); err != nil {
		t.Fatalf("read hold id for %s: %v", taskID, err)
	}
	var status string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT status FROM credit_holds WHERE hold_transaction_id = $1`, holdID).Scan(&status); err != nil {
		t.Fatalf("read hold status for %s: %v", taskID, err)
	}
	return status
}

func (e *atomicEnv) settlementProjection(t *testing.T, taskID string) string {
	t.Helper()
	var state string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT state FROM task_billing_settlements WHERE task_id = $1`, taskID).Scan(&state); err != nil {
		t.Fatalf("read settlement projection for %s: %v", taskID, err)
	}
	return state
}

// seedStalledPaidTask creates a paid task and drives it to the leaked shape:
// claimed (running) with its hold open, then aged past the sweep threshold.
func (e *atomicEnv) seedStalledPaidTask(t *testing.T) *workspacetasks.WorkspaceTask {
	t.Helper()
	created := e.createPaidTask(t, 1)
	e.claimTask(t, created.ID, created.ExecutionToken)
	e.ageTaskPastStallThreshold(t, created.ID, workspacetasks.StalledSettlementThreshold+time.Hour)
	return created
}

// TestStalledSweepReleasesPermanentlyHeldTask is the core fix: the abandoned
// hold is released and the task converges to failed, atomically, exactly like a
// worker-driven failure.
func TestStalledSweepReleasesPermanentlyHeldTask(t *testing.T) {
	env := openAtomicEnv(t, 10)
	stalled := env.seedStalledPaidTask(t)
	// The leak, as it stands before the sweep: running task, held credits.
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("pre-sweep wallet = (%d, %d), want (990, 10)", available, reserved)
	}
	if status := env.holdStatus(t, stalled.ID); status != "held" {
		t.Fatalf("pre-sweep hold status = %q, want held", status)
	}

	report := env.runStalledSweep(t, time.Now())
	if report.Scanned != 1 || report.Converged != 1 || report.Skipped != 0 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want scanned=1 converged=1 no failures", report)
	}

	stored, err := env.store.GetTask(context.Background(), env.ws, stalled.ID)
	if err != nil {
		t.Fatalf("GetTask after sweep: %v", err)
	}
	if stored.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", stored.Status)
	}
	if strings.TrimSpace(stored.ErrorMsg) != workspacetasks.StalledSettlementFailureMessage {
		t.Fatalf("error_msg = %q, want the sweep reason", stored.ErrorMsg)
	}
	if stored.CompletedAt == nil {
		t.Fatal("failed task must carry completed_at (chk_tasks_terminal_consistency)")
	}
	if status := env.holdStatus(t, stalled.ID); status != "released" {
		t.Fatalf("hold status = %q, want released (the leak is what the sweep closes)", status)
	}
	if state := env.settlementProjection(t, stalled.ID); state != "released" {
		t.Fatalf("settlement projection = %q, want released", state)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+stalled.ID) != 1 {
		t.Fatal("release ledger row missing after sweep")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0) (hold fully returned)", available, reserved)
	}
	// The sweep must be indistinguishable from a worker failure on the event
	// channel: a swept task stages exactly one task.failed, post-commit.
	events := env.service.StagedEvents()
	if len(events) != 1 || events[0].Status != workspacetasks.StatusFailed || events[0].TaskID != stalled.ID {
		t.Fatalf("staged events = %+v, want one failed for %s", events, stalled.ID)
	}
}

// TestStalledSweepNeverTouchesWorkAWorkerStillHolds is the anti-false-kill
// proof. A minute-scale threshold would fail this task mid-flight and release
// credits for work that later succeeds: a real money error. A task inside the
// threshold — whether just claimed or merely slow — must be invisible to the
// sweep, hold intact.
func TestStalledSweepNeverTouchesWorkAWorkerStillHolds(t *testing.T) {
	env := openAtomicEnv(t, 10)
	fresh := env.createPaidTask(t, 1)
	env.claimTask(t, fresh.ID, fresh.ExecutionToken)
	nearlyStalled := env.createPaidTask(t, 1)
	env.claimTask(t, nearlyStalled.ID, nearlyStalled.ExecutionToken)
	// Inside the threshold by an hour: "slow", not "abandoned".
	env.ageTaskPastStallThreshold(t, nearlyStalled.ID, workspacetasks.StalledSettlementThreshold-time.Hour)

	report := env.runStalledSweep(t, time.Now())
	if report.Scanned != 0 || report.Converged != 0 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want an empty sweep inside the threshold", report)
	}

	for _, taskID := range []string{fresh.ID, nearlyStalled.ID} {
		stored, err := env.store.GetTask(context.Background(), env.ws, taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		if stored.Status != workspacetasks.StatusRunning {
			t.Fatalf("task %s status = %q, want running (untouched)", taskID, stored.Status)
		}
		if status := env.holdStatus(t, taskID); status != "held" {
			t.Fatalf("task %s hold status = %q, want held (no premature release)", taskID, status)
		}
		if state := env.settlementProjection(t, taskID); state != "held" {
			t.Fatalf("task %s settlement = %q, want held", taskID, state)
		}
		if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+taskID) != 0 {
			t.Fatalf("task %s was released by a sweep inside the threshold", taskID)
		}
	}
	if available, reserved := env.wallet(t); available != 980 || reserved != 20 {
		t.Fatalf("wallet = (%d, %d), want (980, 20) (both holds intact)", available, reserved)
	}
	if events := env.service.StagedEvents(); len(events) != 0 {
		t.Fatalf("staged events = %+v, want none", events)
	}
}

// TestStalledSweepIsIdempotent proves the sweep can run forever without
// double-releasing. Discovery stops naming a converged task, and even if it did
// name one, the workspace-scoped task UPDATE matches zero rows before any
// ledger call — so the second run changes nothing at all.
func TestStalledSweepIsIdempotent(t *testing.T) {
	env := openAtomicEnv(t, 10)
	stalled := env.seedStalledPaidTask(t)

	first := env.runStalledSweep(t, time.Now())
	if first.Converged != 1 {
		t.Fatalf("first sweep = %+v, want converged=1", first)
	}
	ledgerAfterFirst := env.ledgerCount(t, `billing_account_id = $1`, env.accountID)
	// grant + hold + release = 3; a second Release would make it 4.
	if ledgerAfterFirst != 3 {
		t.Fatalf("ledger rows after first sweep = %d, want 3", ledgerAfterFirst)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+stalled.ID) != 1 {
		t.Fatal("release ledger row missing after first sweep")
	}

	second := env.runStalledSweep(t, time.Now())
	if second.Converged != 0 || len(second.Failures) != 0 {
		t.Fatalf("second sweep = %+v, want no convergence and no failures", second)
	}
	if env.ledgerCount(t, `billing_account_id = $1`, env.accountID) != ledgerAfterFirst {
		t.Fatalf("second sweep wrote ledger rows: %d, want %d", env.ledgerCount(t, `billing_account_id = $1`, env.accountID), ledgerAfterFirst)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+stalled.ID) != 1 {
		t.Fatal("second sweep wrote a duplicate Release")
	}
	if status := env.holdStatus(t, stalled.ID); status != "released" {
		t.Fatalf("hold status = %q, want released", status)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
}

// seedSecondWorkspaceWithFundedOwner creates an independent user + personal
// workspace with a funded wallet to test the cross-workspace fence.
func (e *atomicEnv) seedSecondWorkspaceWithFundedOwner(t *testing.T) (workspaceID string, userID int64) {
	t.Helper()
	ctx := context.Background()
	storageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('stalled-other-user', $1, 'user', 1) RETURNING id`,
		storageUID).Scan(&userID); err != nil {
		t.Fatalf("seed second user: %v", err)
	}
	workspaceID = "ws_stalled" + randomAtomicHex(t, 6)
	if _, err := e.pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'stalled-other-ws')`,
		workspaceID, userID); err != nil {
		t.Fatalf("seed second workspace: %v", err)
	}
	if _, err := e.pool.Exec(ctx,
		`INSERT INTO workspace_memberships (workspace_id, user_id, role) SELECT internal_id, $2, 'owner' FROM workspaces WHERE public_id = $1`,
		workspaceID, userID); err != nil {
		t.Fatalf("seed second workspace membership: %v", err)
	}
	var secondWorkspaceInternalID int64
	if err := e.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, workspaceID).Scan(&secondWorkspaceInternalID); err != nil {
		t.Fatalf("resolve second workspace: %v", err)
	}
	secondAccount, err := tenancypg.NewBillingAccountStore(e.pool).EnsureBillingAccountForWorkspace(ctx, secondWorkspaceInternalID, workspaceID)
	if err != nil {
		t.Fatalf("resolve second billing account: %v", err)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.credits.LifecycleInTx(tx).ProvisionAccountWallet(ctx, secondAccount.InternalID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision second wallet: %v", err)
	}
	grant, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := e.credits.InTx(tx).Grant(ctx, ports.GrantCommand{BillingAccountID: secondAccount.InternalID, Amount: grant, Metadata: domain.CommandMetadata{
		IdempotencyKey: "stalled-other-seed-" + randomAtomicHex(t, 4),
		Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "stalled sweep seed",
		BusinessReference: domain.BusinessReference{Type: "test", ID: "stalled-other-seed"},
	}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant second wallet: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return workspaceID, userID
}

// TestStalledSweepNeverSettlesAcrossWorkspaces is the scope proof. Discovery is
// a cross-workspace scan by necessity (an abandoned hold locks SOME user's
// credits), so the guarantee has to come from the write: a (workspace, task)
// pair that does not belong together converges nothing. This mislabels a real
// candidate and proves the wrong pair is inert.
func TestStalledSweepNeverSettlesAcrossWorkspaces(t *testing.T) {
	env := openAtomicEnv(t, 10)
	otherWS, otherUser := env.seedSecondWorkspaceWithFundedOwner(t)
	other, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: otherWS, OwnerUserID: otherUser,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "stalled-other-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("Create in second workspace: %v", err)
	}
	_ = env.service.StagedEvents()
	env.claimTaskIn(t, otherWS, other.ID, other.ExecutionToken)
	env.ageTaskPastStallThreshold(t, other.ID, workspacetasks.StalledSettlementThreshold+time.Hour)

	// A scanner that reports the second workspace's task under the FIRST
	// workspace: the exact shape of a broken join or a hostile row.
	mislabeled := mislabelingScanner{inner: env.store, as: env.ws}
	report, err := workspacetasks.ConvergeStalledSettlements(context.Background(), workspacetasks.StalledConvergenceDeps{
		Scanner:       mislabeled,
		Failer:        env.service,
		StalledBefore: time.Now().Add(-workspacetasks.StalledSettlementThreshold),
		Limit:         workspacetasks.StalledSettlementSweepBatch,
	})
	if err != nil {
		t.Fatalf("mislabeled sweep: %v", err)
	}
	if report.Converged != 0 || report.Skipped != 1 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want converged=0 skipped=1 (nothing settled across workspaces)", report)
	}

	stored, err := env.store.GetTask(context.Background(), otherWS, other.ID)
	if err != nil {
		t.Fatalf("GetTask in second workspace: %v", err)
	}
	if stored.Status != workspacetasks.StatusRunning {
		t.Fatalf("cross-workspace status = %q, want running (the wrong pair must converge nothing)", stored.Status)
	}
	if status := env.holdStatus(t, other.ID); status != "held" {
		t.Fatalf("cross-workspace hold status = %q, want held", status)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+other.ID) != 0 {
		t.Fatal("cross-workspace sweep released a hold")
	}
	var otherWorkspaceInternalID int64
	if err := env.pool.QueryRow(context.Background(), `SELECT internal_id FROM workspaces WHERE public_id = $1`, otherWS).Scan(&otherWorkspaceInternalID); err != nil {
		t.Fatalf("resolve second workspace: %v", err)
	}
	secondAccountID, err := tenancypg.NewBillingAccountStore(env.pool).EnsureBillingAccountForWorkspace(context.Background(), otherWorkspaceInternalID, otherWS)
	if err != nil {
		t.Fatalf("resolve second billing account: %v", err)
	}
	otherWallet, err := env.credits.GetWallet(context.Background(), secondAccountID.InternalID)
	if err != nil {
		t.Fatalf("read second wallet: %v", err)
	}
	if otherWallet.Available != 990 || otherWallet.Reserved != 10 {
		t.Fatalf("second wallet = (%d, %d), want (990, 10) (hold intact)", otherWallet.Available, otherWallet.Reserved)
	}

	// The correctly paired sweep still converges it: the scope guard above is
	// a fail-closed fence, not a filter that swallows real work.
	ok := env.runStalledSweep(t, time.Now())
	if ok.Converged != 1 {
		t.Fatalf("in-scope sweep = %+v, want converged=1", ok)
	}
	if status := env.holdStatus(t, other.ID); status != "released" {
		t.Fatalf("hold status = %q, want released", status)
	}
}

// mislabelingScanner rewrites the workspace of every discovered row. It exists
// to prove the sweep's convergence is fenced by the pair it is handed, not by
// the trust of the scanner.
type mislabelingScanner struct {
	inner workspacetasks.StalledSettlementScanner
	as    string
}

func (m mislabelingScanner) ListStalledHeldSettlements(ctx context.Context, stalledBefore time.Time, limit int) ([]workspacetasks.StalledSettlement, error) {
	rows, err := m.inner.ListStalledHeldSettlements(ctx, stalledBefore, limit)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].WorkspaceID = m.as
	}
	return rows, nil
}

// fakeScanner returns a fixed candidate list and records the arguments it was
// called with.
type fakeScanner struct {
	rows          []workspacetasks.StalledSettlement
	err           error
	calledLimit   int
	calledCutoff  time.Time
	calledCounter int
}

func (f *fakeScanner) ListStalledHeldSettlements(_ context.Context, stalledBefore time.Time, limit int) ([]workspacetasks.StalledSettlement, error) {
	f.calledCounter++
	f.calledLimit, f.calledCutoff = limit, stalledBefore
	return f.rows, f.err
}

// fakeFailer fails or converges per task id.
type fakeFailer struct {
	errs   map[string]error
	calls  []string
	failed map[string]bool
}

func (f *fakeFailer) Fail(_ context.Context, workspaceID, taskID, _ string) (*workspacetasks.WorkspaceTask, error) {
	f.calls = append(f.calls, workspaceID+"/"+taskID)
	if err, ok := f.errs[taskID]; ok {
		return nil, err
	}
	if f.failed == nil {
		f.failed = map[string]bool{}
	}
	f.failed[taskID] = true
	return &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, Status: workspacetasks.StatusFailed}, nil
}

// TestConvergeStalledSettlementsIsolatesPerTaskFailures proves one
// un-releasable task cannot keep every other abandoned hold locked: the batch
// continues past failures and reports them.
func TestConvergeStalledSettlementsIsolatesPerTaskFailures(t *testing.T) {
	ledgerDown := errors.New("ledger unavailable")
	scanner := &fakeScanner{rows: []workspacetasks.StalledSettlement{
		{WorkspaceID: "ws_a", TaskID: "task_a"},
		{WorkspaceID: "ws_b", TaskID: "task_b"},
		{WorkspaceID: "ws_c", TaskID: "task_c"},
	}}
	failer := &fakeFailer{errs: map[string]error{
		"task_b": ledgerDown,
		"task_c": workspacetasks.ErrStateConflict,
	}}
	cutoff := time.Now().Add(-workspacetasks.StalledSettlementThreshold)

	report, err := workspacetasks.ConvergeStalledSettlements(context.Background(), workspacetasks.StalledConvergenceDeps{
		Scanner: scanner, Failer: failer, StalledBefore: cutoff, Limit: 7,
	})
	if err != nil {
		t.Fatalf("ConvergeStalledSettlements: %v", err)
	}
	if report.Scanned != 3 || report.Converged != 1 || report.Skipped != 1 || len(report.Failures) != 1 {
		t.Fatalf("report = %+v, want scanned=3 converged=1 skipped=1 failures=1", report)
	}
	if !errors.Is(report.Failures[0].Err, ledgerDown) || report.Failures[0].Task.TaskID != "task_b" {
		t.Fatalf("failure = %+v, want task_b with the ledger cause preserved", report.Failures[0])
	}
	if !failer.failed["task_a"] {
		t.Fatal("the healthy task must still converge (failures are isolated)")
	}
	if len(failer.calls) != 3 {
		t.Fatalf("Fail calls = %v, want all three attempted", failer.calls)
	}
	if scanner.calledLimit != 7 || !scanner.calledCutoff.Equal(cutoff) {
		t.Fatalf("scanner got limit=%d cutoff=%s, want the caller's 7 and cutoff", scanner.calledLimit, scanner.calledCutoff)
	}
}

// TestConvergeStalledSettlementsFailsClosedOnBadWiring pins the guards: the
// sweep never runs unbounded, and a discovery failure surfaces instead of
// silently converging nothing.
func TestConvergeStalledSettlementsFailsClosedOnBadWiring(t *testing.T) {
	cutoff := time.Now().Add(-workspacetasks.StalledSettlementThreshold)
	scanner := &fakeScanner{}
	failer := &fakeFailer{}

	cases := []struct {
		name string
		deps workspacetasks.StalledConvergenceDeps
		want error
	}{
		{"no scanner", workspacetasks.StalledConvergenceDeps{Failer: failer, StalledBefore: cutoff, Limit: 1}, nil},
		{"no failer", workspacetasks.StalledConvergenceDeps{Scanner: scanner, StalledBefore: cutoff, Limit: 1}, nil},
		{"unbounded limit", workspacetasks.StalledConvergenceDeps{Scanner: scanner, Failer: failer, StalledBefore: cutoff, Limit: 0}, workspacetasks.ErrValidation},
		{"no cutoff", workspacetasks.StalledConvergenceDeps{Scanner: scanner, Failer: failer, Limit: 1}, workspacetasks.ErrValidation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report, err := workspacetasks.ConvergeStalledSettlements(context.Background(), testCase.deps)
			if err == nil {
				t.Fatal("expected a fail-closed error")
			}
			if testCase.want != nil && !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
			if report.Converged != 0 {
				t.Fatalf("report = %+v, want nothing converged", report)
			}
		})
	}

	discoveryDown := errors.New("database unavailable")
	scanner.err = discoveryDown
	if _, err := workspacetasks.ConvergeStalledSettlements(context.Background(), workspacetasks.StalledConvergenceDeps{
		Scanner: scanner, Failer: failer, StalledBefore: cutoff, Limit: 1,
	}); !errors.Is(err, discoveryDown) {
		t.Fatalf("discovery failure = %v, want the cause surfaced", err)
	}
	if len(failer.calls) != 0 {
		t.Fatalf("Fail was called after a discovery failure: %v", failer.calls)
	}
}

// TestStalledSettlementThresholdCannotKillAHealthyTask pins the money-safety
// claim the constant's comment makes. The sweep may only converge a task no
// worker has touched for a full day; the longest healthy execution is one
// River attempt (RiverJobTimeoutDrawing = 15min provider budget + 2min
// overhead), so the threshold must stay orders of magnitude above it. A
// minute-scale bound would release credits for work that later succeeds.
func TestStalledSettlementThresholdCannotKillAHealthyTask(t *testing.T) {
	if workspacetasks.StalledSettlementThreshold < 24*time.Hour {
		t.Fatalf("threshold = %s, want at least 24h", workspacetasks.StalledSettlementThreshold)
	}
	if want := 24 * riverpolicy.RiverJobTimeoutDrawing; workspacetasks.StalledSettlementThreshold < want {
		t.Fatalf("threshold = %s, want >= 24x the longest attempt (%s)", workspacetasks.StalledSettlementThreshold, want)
	}
	// Matches the Credits stale-hold window so the audit signal and the
	// converging action cross at the same threshold.
	if workspacetasks.StalledSettlementThreshold != 24*time.Hour {
		t.Fatalf("threshold = %s, want exactly 24h to match stale_hold_after", workspacetasks.StalledSettlementThreshold)
	}
	if workspacetasks.StalledSettlementSweepBatch <= 0 {
		t.Fatalf("batch = %d, want a positive bound (the scan is never unbounded)", workspacetasks.StalledSettlementSweepBatch)
	}
	if strings.TrimSpace(workspacetasks.StalledSettlementFailureMessage) == "" {
		t.Fatal("the terminal error message must be non-blank for the failed task row")
	}
}
