// Post-commit staging of workspace task status events (#807 dead-channel
// fix).
//
// The settlement contract is: STAGE inside the call, PUBLISH after the
// transaction committed. Staging exists precisely so a publish can never
// announce a state a rollback still has the power to erase. These tests
// pin the other half of that contract over the REAL PostgreSQL settlement
// legs, using a probe transaction over the real store:
//
//   - a staged event means a transaction that committed successfully (the
//     probe records commit-ok, and an independent connection already reads
//     the new status);
//   - a settlement that rolled back, or whose COMMIT ITSELF failed, stages
//     nothing, so no event can escape for a fact that never landed.
//
// The commit-failure leg is the decisive one: it is the only observable
// that separates "staged after the commit" from "staged before the
// commit", because a stage placed ahead of the commit would leave a staged
// event behind on exactly that path.
//
// They run over the same fixture database as settle_atomic_test.go and skip
// without API_TEST_DATABASE_DSN (house convention).
package workspacetasks_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// assertStagedEventIsCommitted reads the task row over a connection
// checked out of the pool — deliberately NOT the settlement transaction —
// so it can only observe what the commit made durable. A staged event whose
// row is still pre-transition would mean staging ran inside the
// transaction.
func assertStagedEventIsCommitted(t *testing.T, env *atomicEnv, taskID string, want workspacetasks.Status) {
	t.Helper()
	conn, err := env.pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire independent connection: %v", err)
	}
	defer conn.Release()
	var status workspacetasks.Status
	if err := conn.QueryRow(context.Background(), `SELECT status FROM tasks WHERE public_id = $1`, taskID).Scan(&status); err != nil {
		t.Fatalf("read committed status for %s: %v", taskID, err)
	}
	if status != want {
		t.Fatalf("staged event observed status %q on an independent connection, want %q (a staged event must follow the commit)", status, want)
	}
}

func assertSingleStagedStatus(t *testing.T, service *workspacetasks.Service, env *atomicEnv, taskID string, want workspacetasks.Status) {
	t.Helper()
	staged := service.StagedEvents()
	if len(staged) != 1 {
		t.Fatalf("staged %d events, want exactly 1: %+v", len(staged), staged)
	}
	if staged[0].Status != want {
		t.Fatalf("staged status = %q, want %q", staged[0].Status, want)
	}
	if staged[0].TaskID != taskID {
		t.Fatalf("staged task_id = %q, want %q", staged[0].TaskID, taskID)
	}
	if staged[0].WorkspaceID != env.ws {
		t.Fatalf("staged workspace_id = %q, want %q", staged[0].WorkspaceID, env.ws)
	}
	if staged[0].OccurredAt.IsZero() {
		t.Fatal("staged event carried a zero occurred_at")
	}
}

// TestSettlementStagesCompletedOnlyAfterCommit proves a successful
// terminal Complete stages exactly one completed event, and that the row
// is already durable when the event exists.
func TestSettlementStagesCompletedOnlyAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events before the terminal settlement, want 0: %+v", len(staged), staged)
	}
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, workspacetasks.StatusCompleted)
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusCompleted)
}

// TestSettlementStagesFailedOnlyAfterCommit proves the failure leg carries
// its own single event, and that a failed Release (which rolls the state
// flip back) stages nothing.
func TestSettlementStagesFailedOnlyAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if _, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, workspacetasks.StatusFailed)
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusFailed)

	// Rollback leg: a Release failure on a second task leaves it running
	// and must not stage anything.
	other := env.createPaidTask(t, 1)
	env.claimTask(t, other.ID, other.ExecutionToken)
	env.holder.setFailures(nil, nil, errors.New("ledger unavailable"))
	if _, err := env.service.Fail(context.Background(), env.ws, other.ID, "provider exploded"); err == nil {
		t.Fatal("Fail with a failing Release must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after a rolled-back Fail, want 0: %+v", len(staged), staged)
	}
	assertStagedEventIsCommitted(t, env, other.ID, workspacetasks.StatusRunning)
}

// TestRetryInTxStagesPendingAfterCommit proves the requeue stages its
// running -> pending event exactly once, that the row is already pending
// when the event exists, and that a failed requeue (which rolls the budget
// bump back) stages nothing.
func TestRetryInTxStagesPendingAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, workspacetasks.StatusPending)
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusPending)

	// Rollback leg: the requeue INSERT rides the settlement transaction,
	// so a River failure rolls the budget bump back and must stage nothing.
	env.queue.failOnce(errors.New("river unavailable"))
	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err == nil {
		t.Fatal("Retry with a failing requeue must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after a rolled-back Retry, want 0: %+v", len(staged), staged)
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask after the rolled-back Retry: %v", err)
	}
	if stored.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1 (the second budget bump rolled back)", stored.RetryCount)
	}
}

// TestRolledBackSettlementLeavesNoEventForTheNextDrain proves the two legs
// do not bleed into each other: after a rolled-back Complete, the next
// committed Complete stages exactly one event (its own). A leak here would
// make the worker's drain publish a transition twice.
func TestRolledBackSettlementLeavesNoEventForTheNextDrain(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	env.holder.setFailures(nil, errors.New("ledger unavailable"), nil)
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err == nil {
		t.Fatal("Complete with a failing Capture must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after the rolled-back Complete, want 0: %+v", len(staged), staged)
	}
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusRunning)

	env.holder.setFailures(nil, nil, nil)
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete after the ledger recovered: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, workspacetasks.StatusCompleted)
}

// TestCancelReplayStagesNoSecondEvent proves exactly-once for the cancel
// leg the admin control adapter drains: a fresh flip stages one cancelled
// event, and a terminal replay stages none (otherwise a repeated operator
// cancel would re-announce the same transition).
func TestCancelReplayStagesNoSecondEvent(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)

	if _, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "operator cancel"); err != nil {
		t.Fatalf("Cancel: %v", err)
	} else if already {
		t.Fatal("first cancel must be a fresh flip")
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, workspacetasks.StatusCancelled)
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusCancelled)

	if _, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "operator cancel"); err != nil {
		t.Fatalf("replayed Cancel: %v", err)
	} else if !already {
		t.Fatal("second cancel must replay as already")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events on a terminal replay, want 0: %+v", len(staged), staged)
	}
}

// probeSettlementStore wraps the real postgres store so the settlement
// transaction it hands out is observable. Nothing about the settlement
// behaviour changes: it is the same store, the same SQL, the same
// transaction — only Commit/Rollback are recorded (and Commit can be made
// to fail on demand). The concrete store is embedded (not the interface)
// so every ...InTx seam is promoted unchanged.
type probeSettlementStore struct {
	*taskpostgres.Store

	mu sync.Mutex
	// pendingFailCommit arms the NEXT transaction to fail its commit, so
	// the injection can be set up before the settlement opens a tx.
	pendingFailCommit error
	steps             []string
	tx                *probeSettlementTx
}

func (s *probeSettlementStore) BeginSettlementTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.Store.BeginSettlementTx(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.steps = append(s.steps, "begin")
	s.tx = &probeSettlementTx{Tx: tx, store: s, failCommit: s.pendingFailCommit}
	s.pendingFailCommit = nil
	s.mu.Unlock()
	return s.tx, nil
}

func (s *probeSettlementStore) record(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
}

func (s *probeSettlementStore) failNextCommit(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		s.tx.failCommit = err
		return
	}
	s.pendingFailCommit = err
}

func (s *probeSettlementStore) sequence() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.steps...)
}

type probeSettlementTx struct {
	pgx.Tx
	store      *probeSettlementStore
	failCommit error
}

func (t *probeSettlementTx) Commit(ctx context.Context) error {
	if t.failCommit != nil {
		t.store.record("commit-fail")
		// The real commit never happens; the caller's deferred rollback
		// then discards every write in the transaction.
		return t.failCommit
	}
	if err := t.Tx.Commit(ctx); err != nil {
		t.store.record("commit-err")
		return err
	}
	t.store.record("commit-ok")
	return nil
}

func (t *probeSettlementTx) Rollback(ctx context.Context) error {
	t.store.record("rollback")
	return t.Tx.Rollback(ctx)
}

// newProbeService builds a second settlement Service over the SAME store,
// holder, quoter, and queue, differing only in that its transactions are
// observable. The staged-event buffer is per-Service, so the assertions
// below read only this Service's buffer.
func newProbeService(t *testing.T, env *atomicEnv) (*workspacetasks.Service, *probeSettlementStore) {
	t.Helper()
	probe := &probeSettlementStore{Store: env.store}
	deps := env.serviceDeps
	deps.TaskLifecycle = probe
	return workspacetasks.NewTestServiceForTests(deps), probe
}

func assertProbeSequence(t *testing.T, probe *probeSettlementStore, want ...string) {
	t.Helper()
	got := probe.sequence()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction steps = %v, want %v", got, want)
	}
}

// TestSettlementStagesNothingWhenTheCommitItselfFails is the decisive
// ordering leg. A staged event is the trigger the caller turns into a
// publish, so a stage that happened BEFORE the commit would announce a
// transition the failed commit erased. Forcing the commit to fail proves
// the stage is not on that side: nothing is staged, the row is untouched,
// and the deferred rollback runs.
func TestSettlementStagesNothingWhenTheCommitItselfFails(t *testing.T) {
	cases := []struct {
		name   string
		settle func(*workspacetasks.Service, context.Context, string, string) error
	}{
		{
			name: "complete",
			settle: func(s *workspacetasks.Service, ctx context.Context, ws, id string) error {
				_, err := s.Complete(ctx, ws, id, atomicRemoteOutput())
				return err
			},
		},
		{
			name: "fail",
			settle: func(s *workspacetasks.Service, ctx context.Context, ws, id string) error {
				_, err := s.Fail(ctx, ws, id, "provider exploded")
				return err
			},
		},
		{
			name: "cancel",
			settle: func(s *workspacetasks.Service, ctx context.Context, ws, id string) error {
				_, _, err := s.Cancel(ctx, ws, id, 1, "operator cancel")
				return err
			},
		},
		{
			name: "retry",
			settle: func(s *workspacetasks.Service, ctx context.Context, ws, id string) error {
				_, err := s.Retry(ctx, ws, id, true)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := openAtomicEnv(t, 10)
			created := env.createPaidTask(t, 1)
			env.claimTask(t, created.ID, created.ExecutionToken)
			service, probe := newProbeService(t, env)
			probe.failNextCommit(errors.New("commit rejected"))

			if err := tc.settle(service, context.Background(), env.ws, created.ID); err == nil {
				t.Fatal("settlement must fail when the commit fails")
			}
			if staged := service.StagedEvents(); len(staged) != 0 {
				t.Fatalf("staged %d events for a failed commit, want 0 (a staged event must follow the commit, never precede it): %+v", len(staged), staged)
			}
			assertProbeSequence(t, probe, "begin", "commit-fail", "rollback")
			// The row never moved (the claim is the only committed fact), so
			// nothing downstream can have observed a transition that does
			// not exist.
			assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusRunning)
		})
	}
}

// TestSettlementStagesOnlyAfterACommittedTransaction proves the positive
// half with the same probe: a staged event is always preceded by a
// successful commit on the settlement's own transaction.
func TestSettlementStagesOnlyAfterACommittedTransaction(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	service, probe := newProbeService(t, env)

	if _, err := service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assertProbeSequence(t, probe, "begin", "commit-ok")
	assertSingleStagedStatus(t, service, env, created.ID, workspacetasks.StatusCompleted)
	assertStagedEventIsCommitted(t, env, created.ID, workspacetasks.StatusCompleted)
}
