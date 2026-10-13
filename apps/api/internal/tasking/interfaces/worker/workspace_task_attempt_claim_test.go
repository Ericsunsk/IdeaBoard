package worker

// Merged claim+attempt seam over the in-memory store, and the two contract
// tests the split claim/StartAttempt pair could not express:
//
//  1. the attempt fact and the lease flip are ONE write, so no execution
//     can hold a lease token without its send fact;
//  2. a settlement failure after a BILLED provider send never re-enters the
//     provider (defect 2).
//
// The single-statement guarantee itself is proven against real PostgreSQL in
// infrastructure/postgres/claim_attempt_test.go; this adapter only mirrors the
// ordering so the worker loop can be exercised without a database.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// attemptClaimStore adapts the in-memory task store to the merged
// claim+attempt seam so unit paths never keep the split claim-then-record
// shape alive. It reproduces the statement's OBSERVABLE contract (a claim is
// only visible once its send fact is written, and a rejected fact write leaves
// the task untouched); the engine-level rollback is proven against real
// PostgreSQL in infrastructure/postgres/claim_attempt_test.go, because
// "UPDATE ... RETURNING + INSERT ... SELECT FROM that CTE roll back together"
// is a PostgreSQL property this double cannot faithfully emulate.
type attemptClaimStore struct {
	*workspacetasks.InMemoryStore
	attempts workspacetasks.AttemptRecorder
	// factErr forces the attempt write to fail, which must take the lease
	// flip with it (the "attempt INSERT rejected" branch of the statement).
	factErr error
	mu      sync.Mutex
	claimed []string
}

var _ WorkspaceTaskAttemptClaimStore = (*attemptClaimStore)(nil)

func newAttemptClaimStore(store *workspacetasks.InMemoryStore, attempts workspacetasks.AttemptRecorder) *attemptClaimStore {
	return &attemptClaimStore{InMemoryStore: store, attempts: attempts}
}

// ClaimTaskWithAttempt mirrors ClaimWorkspaceTaskWithAttempt: the lease flip
// and the send fact become visible together, keyed by the lease token.
func (s *attemptClaimStore) ClaimTaskWithAttempt(ctx context.Context, workspaceID, taskID, leaseToken string, start workspacetasks.AttemptStart) (*workspacetasks.WorkspaceTask, bool, error) {
	if s.factErr != nil {
		// One statement: a failed attempt INSERT rolls the lease flip back,
		// so nothing about this claim may become observable.
		return nil, false, s.factErr
	}
	claimed, err := s.ClaimTask(ctx, workspaceID, taskID, leaseToken)
	if err != nil {
		return nil, false, err
	}
	start.TaskID = taskID
	// The claim is the only writer of the execution token: the attempt row
	// is keyed by the lease the claim just took.
	start.ExecutionToken = strings.TrimSpace(leaseToken)
	if _, err := s.attempts.StartAttempt(ctx, start); err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	s.claimed = append(s.claimed, leaseToken)
	s.mu.Unlock()
	return claimed, true, nil
}

func (s *attemptClaimStore) claimCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.claimed)
}

// faultingSettler reproduces a settlement-layer infrastructure fault:
// Complete fails the way beginSettlementTx fails (DB restart, pool
// exhaustion, or a cancelled execution context), before any state flip.
type faultingSettler struct {
	inner        WorkspaceTaskWorkerSettler
	completeErr  error
	failErr      error
	mu           sync.Mutex
	completeCall int
	failCall     int
}

func (s *faultingSettler) Complete(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	s.mu.Lock()
	s.completeCall++
	s.mu.Unlock()
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	return s.inner.Complete(ctx, workspaceID, taskID, outputs)
}

func (s *faultingSettler) Fail(ctx context.Context, workspaceID, taskID, reason string) (*workspacetasks.WorkspaceTask, error) {
	s.mu.Lock()
	s.failCall++
	s.mu.Unlock()
	if s.failErr != nil {
		return nil, s.failErr
	}
	return s.inner.Fail(ctx, workspaceID, taskID, reason)
}

func (s *faultingSettler) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*workspacetasks.WorkspaceTask, error) {
	return s.inner.Retry(ctx, workspaceID, taskID, retryable)
}

func (s *faultingSettler) calls() (complete, fail int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completeCall, s.failCall
}

// countingProvider records how many times a submission reached the provider,
// which is the number the defect-2 contract caps at one.
type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) Generate(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return workerTestRemoteOutput(), nil
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestWorkerClaimCommitsAttemptFactWithTheLease proves contract 4 at the
// worker level: a successful claim leaves BOTH a running lease and exactly
// one open attempt fact, and that fact carries the lease token (single token
// per execution). Before the merged statement the fact was written by a
// second, independent autocommit with a freshly minted token, so a crash in
// between stranded a running lease with zero attempt rows.
func TestWorkerClaimCommitsAttemptFactWithTheLease(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	worker := env.newWorker(&countingProvider{}, nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}

	started := env.attempts.StartedAttempts(created.ID)
	if len(started) != 1 {
		t.Fatalf("attempt facts = %d, want exactly 1 (claim and attempt commit together)", len(started))
	}
	if started[0].ExecutionToken != created.ExecutionToken {
		t.Fatalf("attempt execution_token = %q, want the lease token %q (one token per execution)",
			started[0].ExecutionToken, created.ExecutionToken)
	}
	if started[0].RetryLayer != workspacetasks.AttemptRetryLayerGateway {
		t.Fatalf("retry_layer = %q, want gateway for the initial schedule", started[0].RetryLayer)
	}
	if started[0].RequestedModel != snap.SelectedModel || started[0].Provider != "openrouter" {
		t.Fatalf("attempt routing facts = %+v, want the snapshot model %q and its provider", started[0], snap.SelectedModel)
	}
	finished := env.attempts.FinishedAttempts(created.ID)
	if len(finished) != 1 || finished[0].Status != workspacetasks.AttemptStatusCompleted {
		t.Fatalf("finished facts = %+v, want one completed fact closed by the lease token", finished)
	}
	if finished[0].ExecutionToken != created.ExecutionToken {
		t.Fatalf("finished under token %q, want the lease token %q", finished[0].ExecutionToken, created.ExecutionToken)
	}
}

// TestWorkerClaimNeverTakesALeaseWithoutItsAttemptFact is the unit twin of
// the PostgreSQL rollback proof: when the attempt write fails, the claim must
// not become observable and the provider must never run, so the forbidden
// "token without an attempt row" state cannot be produced by the worker.
func TestWorkerClaimNeverTakesALeaseWithoutItsAttemptFact(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	env.store.factErr = errors.New("attempt insert rejected")
	provider := &countingProvider{}
	worker := env.newWorker(provider, nil)

	if err := worker.Work(ctx, workerJobFor(created)); err == nil {
		t.Fatal("Work returned nil, want the claim failure surfaced for River backoff")
	}
	if got := provider.count(); got != 0 {
		t.Fatalf("provider calls = %d, want 0 (an unattributable send must never run)", got)
	}
	if facts := env.attempts.StartedAttempts(created.ID); len(facts) != 0 {
		t.Fatalf("attempt facts = %d, want 0 after the rejected write", len(facts))
	}
	if n := env.store.claimCount(); n != 0 {
		t.Fatalf("recorded claims = %d, want 0 (no lease without its fact)", n)
	}
	untouched, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if untouched.Status != workspacetasks.StatusPending {
		t.Fatalf("status = %q, want pending (the lease flip went back with the attempt fact)", untouched.Status)
	}
}

// TestSettlementFailureAfterBilledSendDoesNotReenterProvider is the defect-2
// regression test.
//
// Reproduction: the provider send succeeded (and was billed) and only the
// settlement transaction failed. The old code returned that error, River
// redelivered the SAME job, loadDispatchableTask passed the running row with
// a matching token, the claim's same-token branch re-armed the lease, and the
// provider was called again -- up to RiverMaxAttemptsDefault (25) real,
// billed calls for one submission.
//
// The contract asserted here:
//   - the provider is called exactly once, no matter how often River
//     redelivers;
//   - Work returns nil, so nothing schedules another delivery;
//   - the task converges terminally (hold released) instead of lingering;
//   - tasks.retry_count is untouched: the whole-task retry budget belongs to
//     transient PROVIDER failures (ADR-0010 rule 5), so a settlement fault
//     neither spends it nor depends on the settlement transaction having
//     committed to move it.
func TestSettlementFailureAfterBilledSendDoesNotReenterProvider(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	settler := &faultingSettler{
		inner:       env.service,
		completeErr: errors.New("workspacetasks: begin settlement tx: conn refused"),
	}
	provider := &countingProvider{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler,
		Assets:   &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: provider, Attempts: env.attempts,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}

	job := workerJobFor(created)
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("Work returned %v, want nil: a billed send must not be retried through River", err)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}

	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed (the billed send converges terminally)", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0: the whole-task retry budget is scoped to transient provider failures", got.RetryCount)
	}
	if !strings.Contains(got.ErrorMsg, "settlement failed after provider send") {
		t.Fatalf("error_msg = %q, want the settlement fault recorded", got.ErrorMsg)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, no requeue)", n)
	}

	// River redelivers the same job up to its budget: the task is terminal,
	// so nothing reaches the provider again.
	for i := 0; i < riverpolicy.RiverMaxAttemptsDefault; i++ {
		if err := worker.Work(ctx, workerJobFor(created)); err != nil {
			t.Fatalf("redelivery %d returned %v, want nil", i, err)
		}
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("provider calls after redelivery = %d, want 1 (one submission, one billed send)", got)
	}
	completeCalls, failCalls := settler.calls()
	if completeCalls != 1 || failCalls == 0 {
		t.Fatalf("settler calls = (complete %d, fail %d), want (1, >=1)", completeCalls, failCalls)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (the hold is released on terminal convergence)", released)
	}
}

// TestSettlementFailureWithDeadSettlerStillDoesNotReenterProvider pins the
// deliberate trade in the same contract: when even the terminal write cannot
// commit, Work still returns nil, so River schedules nothing further and the
// task stays running (with its claim token, its durable attempt fact, and its
// hold, so the state is fully attributable and an operator can settle it)
// rather than being handed to River, whose only recovery is another billed
// provider call.
//
// Documented residual: a HARD process crash after the provider returned but
// before this code runs still costs one extra send, because the redelivery
// legitimately reacquires its own lease (the claim's same-token branch) and
// finishes the send. That is bounded at two sends per submission and is
// preferred over stranding a paid-for task; the 25-call loop came from the
// retryable error this path no longer returns.
func TestSettlementFailureWithDeadSettlerStillDoesNotReenterProvider(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	settler := &faultingSettler{
		inner:       env.service,
		completeErr: errors.New("workspacetasks: begin settlement tx: conn refused"),
		failErr:     errors.New("workspacetasks: begin settlement tx: conn refused"),
	}
	provider := &countingProvider{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler,
		Assets:   &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: provider, Attempts: env.attempts,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work returned %v, want nil (never re-enter the provider)", err)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusRunning {
		t.Fatalf("status = %q, want running (no terminal write could commit)", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0 (a settlement fault spends no provider retry)", got.RetryCount)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only: nothing was requeued for River to retry)", n)
	}
	if facts := env.attempts.StartedAttempts(created.ID); len(facts) != 1 {
		t.Fatalf("attempt facts = %d, want 1 (the billed send stays attributable)", len(facts))
	}
	// The terminal write was retried on a detached context before giving up.
	if _, fails := settler.calls(); fails < 2 {
		t.Fatalf("Fail calls = %d, want >=2 (the terminal write retries inside its detached budget)", fails)
	}
}

// TestWorkspaceAttemptLayer pins the retry-layer attribution. retry_count
// alone was wrong: the claim's same-token branch lets a River redelivery
// re-acquire a lease without incrementing retry_count, so those sends were
// recorded as fresh gateway sends.
func TestWorkspaceAttemptLayer(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryCount int32
		jobAttempt int
		want       string
	}{
		{"initial schedule", 0, 0, workspacetasks.AttemptRetryLayerGateway},
		{"after a worker requeue rotated the token", 1, 0, workspacetasks.AttemptRetryLayerRiverTask},
		{"river redelivery of the same job", 0, 1, workspacetasks.AttemptRetryLayerRiverTask},
		{"redelivery after a requeue", 2, 3, workspacetasks.AttemptRetryLayerRiverTask},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workspaceAttemptLayer(tc.retryCount, tc.jobAttempt); got != tc.want {
				t.Fatalf("layer = %q, want %q", got, tc.want)
			}
		})
	}
}
