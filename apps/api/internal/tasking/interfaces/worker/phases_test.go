package worker

// Phase-boundary tests (#860): Claim, Execution, and Retry+Settlement
// state transitions verified independently of the full Work loop. The
// loop-level twins live in worker_test.go (success,
// deterministic failure, requeue, exhaustion) and
// snapshot_fallback_test.go (fallback at-most-once,
// taxonomy); these tests pin the seams between phases.

import (
	"context"
	"fmt"
	"testing"

	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"
)

// A job carrying no execution token fails closed (errors for River
// backoff, provider never runs, task stays pending): Create/Retry always
// mint the lease, so an empty lease only arrives on a hand-made job and
// must never mint silently.
func TestWorkspaceTaskClaimFailsClosedWhenJobTokenEmpty(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(_ context.Context, ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)

	job := workerJobFor(created)
	job.Args.ExecutionToken = ""
	if err := worker.Work(ctx, job); err == nil {
		t.Fatal("Work returned nil, want the missing-lease error surfaced for River backoff")
	}
	if called {
		t.Fatal("provider called, want no execution without a fenced lease")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusPending {
		t.Fatalf("status = %q, want pending (unclaimed, hold untouched)", got.Status)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, no requeue)", n)
	}
}

// A provider send yielding more than MaxOutputsPerTask refs is a
// deterministic execution failure: terminal fail, full hold released, no
// retry burn.
func TestWorkspaceTaskGenerateRejectsOversizeOutputs(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	refs := make([]tasks.OutputRef, 0, tasks.MaxOutputsPerTask+1)
	for i := 0; i <= tasks.MaxOutputsPerTask; i++ {
		refs = append(refs, tasks.OutputRef{
			Kind: tasks.OutputKindRemote, URL: fmt.Sprintf("https://cdn.example.com/oversize-%d.png", i),
			ChecksumSHA256: workerTestRemoteChecksum, Role: "result", ResultIndex: i,
		})
	}
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(_ context.Context, ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
		return refs, nil
	}), nil)

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusFailed {
		t.Fatalf("status = %q, want failed for oversize outputs", got.Status)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back, no retry burn)", released)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, no requeue)", n)
	}
}

// phaseStubSettler records terminal-settlement calls for the
// retry-vs-terminal matrix below.
type phaseStubSettler struct {
	completeCalls int
	failCalls     int
	retryCalls    int
}

func (s *phaseStubSettler) Complete(_ context.Context, _, _ string, _ []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.completeCalls++
	return &tasks.WorkspaceTask{}, nil
}

func (s *phaseStubSettler) Fail(_ context.Context, _, _, _ string) (*tasks.WorkspaceTask, error) {
	s.failCalls++
	return &tasks.WorkspaceTask{}, nil
}

func (s *phaseStubSettler) Retry(_ context.Context, _, _ string, _ bool) (*tasks.WorkspaceTask, error) {
	s.retryCalls++
	return &tasks.WorkspaceTask{}, nil
}

// phaseStubStore satisfies the constructor; the converge path never reads.
type phaseStubStore struct{}

func (phaseStubStore) GetTask(_ context.Context, _, _ string) (*tasks.WorkspaceTask, error) {
	return nil, tasks.ErrNotFound
}

func (phaseStubStore) ClaimTask(_ context.Context, _, _, _ string) (*tasks.WorkspaceTask, error) {
	return nil, tasks.ErrStateConflict
}

// ClaimTaskWithAttempt satisfies the merged claim seam the constructor
// requires; this stub never claims, so it never opens an attempt fact.
func (phaseStubStore) ClaimTaskWithAttempt(_ context.Context, _, _, _ string, _ tasks.AttemptStart) (*tasks.WorkspaceTask, bool, error) {
	return nil, false, tasks.ErrStateConflict
}

// convergeSendFailure routes purely on (retryable, budget): retryable with
// budget requeues through Retry, exhausted budget or deterministic failures
// converge through Fail. No store, no provider, no River involved.
func TestConvergeSendFailureRetryVsTerminalMatrix(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		failure   routing.AttemptFailure
		retryCnt  int
		wantRetry int
		wantFail  int
	}{
		{"transient with budget requeues", routing.AttemptFailure{HTTPStatus: 503}, 0, 1, 0},
		{"network error with budget requeues", routing.AttemptFailure{NetworkError: true}, 1, 1, 0},
		{"transient with exhausted budget fails", routing.AttemptFailure{HTTPStatus: 503}, tasks.MaxProviderRequeues, 0, 1},
		{"deterministic with budget fails", routing.AttemptFailure{InvalidParams: true}, 0, 0, 1},
		{"unmarked failure fails closed", routing.AttemptFailure{}, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settler := &phaseStubSettler{}
			worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{Store: phaseStubStore{}, Settler: settler})
			if err != nil {
				t.Fatalf("NewWorkspaceTaskWorker: %v", err)
			}
			claimed := &tasks.WorkspaceTask{
				ID: "task_phase1", WorkspaceID: "ws_test", Service: "render", RetryCount: tc.retryCnt,
			}
			send := newWorkspaceSendTracker(nil, claimed.ID, 0, "render").
				startSend(ctx, "openrouter/model-a", tasks.AttemptRetryLayerGateway)
			genErr := fmt.Errorf("provider send failed: %s", tc.name)
			if err := worker.convergeSendFailure(ctx, send, claimed, tc.failure, genErr, false); err != nil {
				t.Fatalf("convergeSendFailure: %v", err)
			}
			if settler.retryCalls != tc.wantRetry || settler.failCalls != tc.wantFail {
				t.Fatalf("retry/fail = (%d, %d), want (%d, %d)",
					settler.retryCalls, settler.failCalls, tc.wantRetry, tc.wantFail)
			}
			if settler.completeCalls != 0 {
				t.Fatalf("complete calls = %d, want 0 (failure path never completes)", settler.completeCalls)
			}
		})
	}
}

// failTerminal on an already-converged task drops the job (nil) instead of
// erroring: a cancel landing mid-flight must not ride River backoff.
func TestFailTerminalDropsConvergedTask(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if _, err := env.service.Fail(ctx, "ws_test", created.ID, "first failure"); err != nil {
		t.Fatalf("service Fail: %v", err)
	}
	worker := env.newWorker(nil, nil)
	current, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := worker.failTerminal(ctx, current, "late failure after converge"); err != nil {
		t.Fatalf("failTerminal on converged task = %v, want drop nil", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusFailed {
		t.Fatalf("status = %q, want failed (unchanged)", got.Status)
	}
}
